package provider

import (
	"context"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/schema/errors"
	"net"
	"sync"
	"time"
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
}

func NewAmqpProvider(amqpConfig *config.AMQPConfig) AmqpProviderInterface {
	return &amqpProvider{amqpConf: amqpConfig}
}

func (ap *amqpProvider) CreateAmqpChannel(conn *amqp.Connection, confirm bool) (*amqp.Channel, chan amqp.Confirmation, error) {
	channel, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	if confirm && channel != nil {
		if err = channel.Confirm(false); err != nil {
			return nil, nil, err
		}
		return channel, channel.NotifyPublish(make(chan amqp.Confirmation, 1)), err
	}

	return channel, nil, err
}

func (ap *amqpProvider) CloseAmqpChannel(channel *amqp.Channel) error {
	return channel.Close()
}

func (ap *amqpProvider) CreateConsumer(channel *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error) {
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

func (ap *amqpProvider) AmqpPublishWithConfirm(ctx context.Context, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error {

	// Get a connection from the pool
	conn, err := ap.GetAmqpConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider AmqpProviderInterface, i interface{}) {
		err := ap.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(ap, conn)

	// Create a channel
	channel, confirmChan, err := ap.CreateAmqpChannel(conn, true)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := ap.CloseAmqpChannel(channel)
		if err != nil {
			//TODO:error handling
		}
	}(channel)

	// Publish the message to the exchange
	err = channel.PublishWithContext(ctx,
		exchangeName, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqpMsg,
	)

	// wait for confirm with timeout
	select {
	case confirmed, ok := <-confirmChan:
		if !ok {
			return fmt.Errorf("publish confirm channel closed")
		}
		if confirmed.Ack {
			return nil
		}
		return fmt.Errorf("message not acked by broker")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("publish confirm timeout")
	}
}

func (ap *amqpProvider) GetAmqpConnection() (*amqp.Connection, error) {
	amqpConn, err := ap.GetConnectionFromPool()
	if err != nil {
		return nil, err
	}
	if amqpConnection, ok := amqpConn.(*amqp.Connection); ok {
		return amqpConnection, nil
	}
	return nil, nil
}

func (ap *amqpProvider) SetChannelQoS(channel *amqp.Channel, prefetchCount int) error {
	return channel.Qos(
		prefetchCount,
		0,     // prefetch size
		false, // global
	)
}

func (ap *amqpProvider) CloseConnection() error {
	// Implement AMQP connection closing logic
	return nil
}

/*
func (ap *amqpProvider) CreateConnection() (*amqp.Connection, error) {
	if ap.amqpConf == nil {
		return nil, errors.ErrNilConfig
	}
	conn, err := amqp.DialConfig(ap.amqpConf.Url,
		amqp.Config{
			Heartbeat: time.Duration(ap.amqpConf.HeartBeatInterval), // Adjust heartbeat interval as needed
		},
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}*/

func (ap *amqpProvider) CreateConnection() (*amqp.Connection, error) {
	if ap.amqpConf == nil {
		return nil, errors.ErrNilConfig
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

// CreateConnectionPool initializes the connection pool safely.
func (ap *amqpProvider) CreateConnectionPool() error {
	poolSize := ap.amqpConf.ConnectionPoolSize
	if poolSize < 1 {
		return errors.ErrInvalidConfig
	}

	// Allocate pool slice
	connPool := make([]*amqp.Connection, poolSize)

	// Create all connections first (NO watchers here)
	for i := 0; i < poolSize; i++ {
		newConn, err := ap.CreateConnection()
		if err != nil {
			// Cleanup previously created connections
			for _, c := range connPool {
				if c != nil {
					_ = c.Close()
				}
			}
			return fmt.Errorf("CreateConnectionPool failed: %w", err)
		}

		connPool[i] = newConn
	}

	// Lock and assign entire pool at once (atomic)
	ap.connectionsMutex.Lock()
	ap.ConnectionPool = connPool
	ap.connectionsMutex.Unlock()

	// AFTER pool assignment → start watchers
	for _, conn := range connPool {
		ap.watchConnection(conn)
	}

	return nil
}

func (ap *amqpProvider) watchConnection(conn *amqp.Connection) {
	go func() {
		err := <-conn.NotifyClose(make(chan *amqp.Error))
		if err != nil {
			ap.replaceDeadConnection(conn)
		}
	}()
}

func (ap *amqpProvider) replaceDeadConnection(dead *amqp.Connection) {
	ap.connectionsMutex.Lock()
	defer ap.connectionsMutex.Unlock()
	if ap.ConnectionPool == nil {
		return
	}
	for i, c := range ap.ConnectionPool {
		if c == nil {
			continue
		}
		if c == dead {
			newConn, err := ap.CreateConnection()
			if err != nil {
				// keeping the dead one, will retry on next get
				return
			}

			ap.ConnectionPool[i] = newConn
			ap.watchConnection(newConn)
			return
		}
	}
}

// ReleaseConnectionToPool releases a connection back to the pool
func (ap *amqpProvider) ReleaseConnectionToPool(conn interface{}) error {

	if amqpConnection, ok := conn.(*amqp.Connection); ok {

		ap.connectionsMutex.Lock()
		defer ap.connectionsMutex.Unlock()

		// If dead → create new one and add that instead
		if amqpConnection == nil || amqpConnection.IsClosed() {
			newConn, err := ap.CreateConnection()
			if err != nil {
				return fmt.Errorf("failed to recreate AMQP connection: %w", err)
			}
			ap.watchConnection(amqpConnection)
			ap.ConnectionPool = append(ap.ConnectionPool, newConn)
			return nil
		}

		ap.ConnectionPool = append(ap.ConnectionPool, amqpConnection)
	}
	return nil
}

// GetConnectionFromPool returns a valid AMQP connection from the pool.
// It does NOT append it back (ReleaseConnectionToPool will handle that).
func (ap *amqpProvider) GetConnectionFromPool() (interface{}, error) {
	ap.connectionsMutex.Lock()
	defer ap.connectionsMutex.Unlock()

	if ap.ConnectionPool == nil || len(ap.ConnectionPool) == 0 {
		return nil, errors.ErrConnectionPoolEmpty
	}

	// Pop first connection
	amqpConn := ap.ConnectionPool[0]
	ap.ConnectionPool = ap.ConnectionPool[1:]

	//If connection is closed → create a new one and return it
	if amqpConn == nil || amqpConn.IsClosed() {

		newAmqpConn, err := ap.CreateConnection()
		if err != nil {
			return nil, fmt.Errorf("failed to recreate AMQP connection: %w", err)
		}
		ap.watchConnection(newAmqpConn)
		// DO NOT append here — ReleaseConnectionToPool will manage that
		return newAmqpConn, nil
	}

	// Return healthy connection (not appended)
	return amqpConn, nil
}
