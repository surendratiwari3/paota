package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/utils"
	"github.com/surendratiwari3/paota/logger"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

const (
	// defaultPublishConfirmTimeout bounds how long a publisher waits for the
	// broker to confirm a message before the connection is considered bad.
	defaultPublishConfirmTimeout = 5 * time.Second
	// publishAttempts is the number of times AmqpPublishWithConfirm will try a
	// publish. A pooled connection can be closed by the broker while it sits
	// idle, and the failure only surfaces when it is next used; retrying with a
	// freshly dialled connection turns that into a transparent recovery instead
	// of a lost task.
	publishAttempts = 3
	// defaultConnectionPoolSize is used when the configuration does not specify
	// a pool size, so releases still have a bound to check against.
	defaultConnectionPoolSize = 1
)

type AmqpProviderInterface interface {
	AmqpPublish(ctx context.Context, channel *amqp.Channel, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error
	AmqpPublishWithConfirm(ctx context.Context, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error
	CreateAmqpChannel(conn *amqp.Connection, confirm bool) (*amqp.Channel, chan amqp.Confirmation, error)
	CloseAmqpChannel(channel *amqp.Channel) error
	CloseConnection() error
	CreateConnectionPool() error
	CreateConsumer(channel *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error)
	DeclareQueue(channel *amqp.Channel, queueName string, declareQueueArgs amqp.Table) error
	DeclareExchange(channel *amqp.Channel, exchangeName string, exchangeType string) error
	GetConnectionFromPool() (interface{}, error)
	QueueExchangeBind(channel *amqp.Channel, queueName string, routingKey string, exchangeName string) error
	ReleaseConnectionToPool(interface{}) error
	SetChannelQoS(channel *amqp.Channel, prefetchCount int) error
}

type amqpProvider struct {
	amqpConf         *config.AMQPConfig
	ConnectionPool   []*amqp.Connection
	connectionsMutex sync.Mutex
	closed           bool
}

func NewAmqpProvider(amqpConfig *config.AMQPConfig) AmqpProviderInterface {
	return &amqpProvider{amqpConf: amqpConfig}
}

func (ap *amqpProvider) CreateAmqpChannel(conn *amqp.Connection, confirm bool) (*amqp.Channel, chan amqp.Confirmation, error) {
	if conn == nil {
		return nil, nil, appErrors.ErrInvalidConnectionPool
	}

	channel, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	if confirm && channel != nil {
		if err = channel.Confirm(false); err != nil {
			// Do not leak the channel we just opened.
			_ = channel.Close()
			return nil, nil, err
		}
		return channel, channel.NotifyPublish(make(chan amqp.Confirmation, 1)), nil
	}

	return channel, nil, nil
}

func (ap *amqpProvider) CloseAmqpChannel(channel *amqp.Channel) error {
	if channel == nil {
		return nil
	}
	err := channel.Close()
	// Closing a channel whose connection is already gone is not an error worth
	// propagating during shutdown.
	if errors.Is(err, amqp.ErrClosed) {
		return nil
	}
	return err
}

func (ap *amqpProvider) CreateConsumer(channel *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error) {
	if channel == nil {
		return nil, appErrors.ErrInvalidConnectionPool
	}
	return channel.Consume(
		queueName,   // queue
		consumerTag, // consumer tag
		false,       // auto-ack
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // arguments
	)
}

func (ap *amqpProvider) DeclareExchange(channel *amqp.Channel, exchangeName string, exchangeType string) error {
	return channel.ExchangeDeclare(
		exchangeName, // exchange name
		exchangeType, // exchange type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
}

func (ap *amqpProvider) DeclareQueue(channel *amqp.Channel, queueName string, declareQueueArgs amqp.Table) error {
	_, err := channel.QueueDeclare(
		queueName,        // queue name
		true,             // durable
		false,            // delete when unused
		false,            // exclusive
		false,            // no-wait
		declareQueueArgs, // arguments
	)
	return err
}

func (ap *amqpProvider) QueueExchangeBind(channel *amqp.Channel, queueName string, routingKey string, exchangeName string) error {
	// Bind the queue to the exchange
	return channel.QueueBind(
		queueName,    // queue name
		routingKey,   // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
}

func (ap *amqpProvider) AmqpPublish(ctx context.Context, channel *amqp.Channel, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error {
	if channel == nil {
		return appErrors.ErrInvalidConnectionPool
	}

	// Publish the message to the exchange
	err := channel.PublishWithContext(ctx,
		exchangeName, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqpMsg,
	)

	return err
}

// DiscardConnection closes a connection instead of returning it to the pool.
func (ap *amqpProvider) DiscardConnection(conn interface{}) error {
	// actually close it
	if c, ok := conn.(*amqp.Connection); ok && c != nil {
		_ = c.Close()
	}

	// do not return to pool → it gets dropped
	return nil
}

// AmqpPublishWithConfirm publishes a message and waits for the broker to
// confirm it. A failed attempt discards the connection and retries on a fresh
// one, so a publish that races with a broker restart is not silently lost.
func (ap *amqpProvider) AmqpPublishWithConfirm(ctx context.Context, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error {
	var lastErr error

	for attempt := 0; attempt < publishAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return fmt.Errorf("%w (last publish error: %v)", err, lastErr)
			}
			return err
		}

		if attempt > 0 {
			// Small backoff so a broker that is still coming back up is not
			// hammered by every publisher at once.
			utils.JitterMsecSleep(50*attempt, 150*attempt)
		}

		err := ap.publishOnce(ctx, routingKey, amqpMsg, exchangeName)
		if err == nil {
			return nil
		}
		lastErr = err
		logger.ApplicationLogger.Warning("publish attempt failed, retrying: ", err)
	}

	return fmt.Errorf("publish failed after %d attempts: %w", publishAttempts, lastErr)
}

func (ap *amqpProvider) publishOnce(ctx context.Context, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error {
	// Get a connection from the pool
	conn, err := ap.GetAmqpConnection()
	if err != nil {
		return err
	}

	// Track if connection should be discarded
	publishHadError := false

	// Return connection or discard it depending on result
	defer func() {
		if publishHadError {
			_ = ap.DiscardConnection(conn) // REMOVE bad connection from pool
		} else {
			_ = ap.ReleaseConnectionToPool(conn) // Return good connection to pool
		}
	}()

	// Create channel with confirms enabled
	channel, confirmChan, err := ap.CreateAmqpChannel(conn, true)
	if err != nil {
		publishHadError = true
		return err
	}
	defer func() {
		_ = ap.CloseAmqpChannel(channel)
	}()

	// Publish
	err = channel.PublishWithContext(ctx,
		exchangeName,
		routingKey,
		false,
		false,
		amqpMsg,
	)

	if err != nil {
		publishHadError = true
		return fmt.Errorf("failed to publish: %w", err)
	}

	// Wait for broker confirm
	timer := time.NewTimer(ap.publishConfirmTimeout())
	defer timer.Stop()

	select {
	case confirmed, ok := <-confirmChan:
		if !ok {
			publishHadError = true
			return fmt.Errorf("publish confirm channel closed (connection probably reset)")
		}

		if !confirmed.Ack {
			publishHadError = true
			return fmt.Errorf("message not acked by broker")
		}

		// Confirm OK → connection is healthy
		return nil

	case <-ctx.Done():
		publishHadError = true
		return fmt.Errorf("publish confirm aborted: %w", ctx.Err())

	case <-timer.C:
		publishHadError = true
		return fmt.Errorf("publish confirm timeout")
	}
}

func (ap *amqpProvider) publishConfirmTimeout() time.Duration {
	if ap.amqpConf != nil && ap.amqpConf.PublishConfirmTimeout > 0 {
		return time.Duration(ap.amqpConf.PublishConfirmTimeout) * time.Second
	}
	return defaultPublishConfirmTimeout
}

func (ap *amqpProvider) GetAmqpConnection() (*amqp.Connection, error) {
	amqpConn, err := ap.GetConnectionFromPool()
	if err != nil {
		return nil, err
	}
	amqpConnection, ok := amqpConn.(*amqp.Connection)
	if !ok || amqpConnection == nil {
		// Returning (nil, nil) here made every caller dereference a nil
		// connection.
		return nil, appErrors.ErrInvalidConnectionPool
	}
	return amqpConnection, nil
}

func (ap *amqpProvider) SetChannelQoS(channel *amqp.Channel, prefetchCount int) error {
	if channel == nil {
		return appErrors.ErrInvalidConnectionPool
	}
	if prefetchCount < 0 {
		prefetchCount = 0
	}
	return channel.Qos(
		prefetchCount,
		0,     // prefetch size
		false, // global
	)
}

// CloseConnection closes every pooled connection and marks the provider as
// closed. It is idempotent and safe to call during shutdown.
func (ap *amqpProvider) CloseConnection() error {
	ap.connectionsMutex.Lock()
	pool := ap.ConnectionPool
	ap.ConnectionPool = nil
	ap.closed = true
	ap.connectionsMutex.Unlock()

	var firstErr error
	for _, conn := range pool {
		if conn == nil || conn.IsClosed() {
			continue
		}
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (ap *amqpProvider) CreateConnection() (*amqp.Connection, error) {
	if ap.amqpConf == nil {
		return nil, appErrors.ErrNilConfig
	}

	// Heartbeat (convert seconds → time.Duration)
	heartbeat := time.Duration(ap.amqpConf.HeartBeatInterval) * time.Second
	if heartbeat <= 0 {
		heartbeat = 10 * time.Second
	}

	// Connection timeout
	dialTimeout := time.Duration(ap.amqpConf.ConnectionTimeout) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	// Build AMQP config
	amqpCfg := amqp.Config{
		Heartbeat: heartbeat,
		Locale:    "en_US",
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, dialTimeout)
		},
	}

	// Dial broker
	conn, err := amqp.DialConfig(ap.amqpConf.Url, amqpCfg)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// poolSize returns the configured pool size, defaulting to a single connection
// when the configuration does not specify one.
func (ap *amqpProvider) poolSize() int {
	if ap.amqpConf == nil || ap.amqpConf.ConnectionPoolSize < 1 {
		return defaultConnectionPoolSize
	}
	return ap.amqpConf.ConnectionPoolSize
}

// CreateConnectionPool initializes the connection pool safely.
func (ap *amqpProvider) CreateConnectionPool() error {
	if ap.amqpConf == nil {
		return appErrors.ErrNilConfig
	}

	if ap.amqpConf.ConnectionPoolSize < 1 {
		return appErrors.ErrInvalidConfig
	}
	poolSize := ap.poolSize()

	// Allocate pool slice
	connPool := make([]*amqp.Connection, poolSize)

	// Create all connections first (NO watchers here)
	for i := 0; i < poolSize; i++ {
		newConn, err := ap.CreateConnection()
		if err != nil {
			// Cleanup previously created connections
			closeAll(connPool)
			return fmt.Errorf("CreateConnectionPool failed: %w", err)
		}

		connPool[i] = newConn
	}

	// Lock and assign entire pool at once (atomic)
	ap.connectionsMutex.Lock()
	if ap.closed {
		ap.connectionsMutex.Unlock()
		closeAll(connPool)
		return appErrors.ErrConnectionPoolClosed
	}
	// Calling CreateConnectionPool twice used to abandon the previous
	// connections without closing them.
	previous := ap.ConnectionPool
	ap.ConnectionPool = connPool
	ap.connectionsMutex.Unlock()

	closeAll(previous)

	// AFTER pool assignment → start watchers
	for _, conn := range connPool {
		ap.watchConnection(conn)
	}

	return nil
}

func closeAll(conns []*amqp.Connection) {
	for _, c := range conns {
		if c != nil && !c.IsClosed() {
			_ = c.Close()
		}
	}
}

// watchConnection drops a connection from the pool as soon as the broker
// reports it closed. Replacements are dialled lazily by
// GetConnectionFromPool, so a broker outage does not turn into a reconnect
// storm from background goroutines.
func (ap *amqpProvider) watchConnection(conn *amqp.Connection) {
	if conn == nil {
		return
	}
	go func() {
		if err := <-conn.NotifyClose(make(chan *amqp.Error, 1)); err != nil {
			logger.ApplicationLogger.Warning("amqp connection closed, removing from pool: ", err)
		}
		ap.removeConnection(conn)
	}()
}

func (ap *amqpProvider) removeConnection(dead *amqp.Connection) {
	ap.connectionsMutex.Lock()
	defer ap.connectionsMutex.Unlock()

	for i, c := range ap.ConnectionPool {
		if c == dead {
			ap.ConnectionPool = append(ap.ConnectionPool[:i], ap.ConnectionPool[i+1:]...)
			return
		}
	}
}

// ReleaseConnectionToPool releases a connection back to the pool.
// Dead connections are dropped rather than recycled, and the pool never grows
// beyond the configured size.
func (ap *amqpProvider) ReleaseConnectionToPool(conn interface{}) error {
	amqpConnection, ok := conn.(*amqp.Connection)
	if !ok || amqpConnection == nil {
		return nil
	}

	if amqpConnection.IsClosed() {
		// Nothing to recycle. A replacement is dialled on demand by
		// GetConnectionFromPool; the old code created one here and then
		// registered the close-watcher on the *dead* connection, so the
		// replacement was never monitored.
		return nil
	}

	ap.connectionsMutex.Lock()
	if ap.closed {
		ap.connectionsMutex.Unlock()
		_ = amqpConnection.Close()
		return nil
	}
	// Bound the pool: releases used to append unconditionally, so a busy
	// publisher grew the pool without limit.
	if len(ap.ConnectionPool) >= ap.poolSize() {
		ap.connectionsMutex.Unlock()
		_ = amqpConnection.Close()
		return nil
	}
	ap.ConnectionPool = append(ap.ConnectionPool, amqpConnection)
	ap.connectionsMutex.Unlock()

	return nil
}

// GetConnectionFromPool returns a valid AMQP connection from the pool.
// It does NOT append it back (ReleaseConnectionToPool will handle that).
func (ap *amqpProvider) GetConnectionFromPool() (interface{}, error) {
	ap.connectionsMutex.Lock()
	if ap.closed {
		ap.connectionsMutex.Unlock()
		return nil, appErrors.ErrConnectionPoolClosed
	}

	// Pop the first healthy connection, dropping any that died while idle.
	for len(ap.ConnectionPool) > 0 {
		conn := ap.ConnectionPool[0]
		ap.ConnectionPool = ap.ConnectionPool[1:]

		if conn != nil && !conn.IsClosed() {
			ap.connectionsMutex.Unlock()
			return conn, nil
		}
	}
	ap.connectionsMutex.Unlock()

	// Pool is empty (first use, or the broker went away). Dial outside the lock
	// so a slow or failing dial does not block every other caller.
	newConn, err := ap.CreateConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create AMQP connection: %w", err)
	}

	ap.connectionsMutex.Lock()
	closedNow := ap.closed
	ap.connectionsMutex.Unlock()
	if closedNow {
		_ = newConn.Close()
		return nil, appErrors.ErrConnectionPoolClosed
	}

	ap.watchConnection(newConn)
	// DO NOT append here — ReleaseConnectionToPool will manage that
	return newConn, nil
}
