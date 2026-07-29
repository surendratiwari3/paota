package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/schema/errors"
)

// AMQPBroker represents an AMQP broker
type AMQPBroker struct {
	config           *config.Config
	ackChannel       chan uint64
	connectionsMutex sync.Mutex
	processingWG     sync.WaitGroup
	amqpErrorChannel <-chan *amqp.Error
	stopChannel      chan struct{}
	doneStopChannel  chan struct{}
	amqpProvider     provider.AmqpProviderInterface
	// consuming guards StopConsumer so it can never block on a consumer that
	// is not running.
	consuming atomic.Bool
}

// globalAmqpProvider defined just for unit test cases
var (
	globalAmqpProvider provider.AmqpProviderInterface
)

func (b *AMQPBroker) getConnection() (*amqp.Connection, error) {
	amqpConn, err := b.amqpProvider.GetConnectionFromPool()
	if err != nil {
		return nil, err
	}
	amqpConnection, ok := amqpConn.(*amqp.Connection)
	if !ok || amqpConnection == nil {
		// Returning (nil, nil) here used to push a nil connection into
		// callers that immediately dereferenced it.
		return nil, errors.ErrInvalidConnectionPool
	}
	return amqpConnection, nil
}

// getRoutingKey gets the routing key from the signature
func (b *AMQPBroker) getRoutingKey() string {
	if b.config.AMQP == nil || b.config.AMQP.BindingKey == "" {
		return b.getTaskQueue()
	}
	if b.isDirectExchange() {
		return b.config.AMQP.BindingKey
	}

	return b.getTaskQueue()
}

// isDirectExchange checks if the exchange type is direct
func (b *AMQPBroker) isDirectExchange() bool {
	return b.config.AMQP != nil && b.config.AMQP.ExchangeType == "direct"
}

// NewAMQPBroker creates a new instance of the AMQP broker
// It opens connections to RabbitMQ, declares an exchange, opens a channel,
// declares and binds the queue, and enables publish notifications
func NewAMQPBroker(configProvider config.ConfigProvider) (broker.Broker, error) {
	if configProvider == nil {
		return nil, errors.ErrNilConfig
	}

	cfg := configProvider.GetConfig()
	// GetConfig returns nil when no configuration was ever set; every accessor
	// below dereferences cfg.AMQP, so fail loudly instead of panicking.
	if cfg == nil || cfg.AMQP == nil {
		return nil, errors.ErrNilConfig
	}

	amqpErrorChannel := make(chan *amqp.Error, 1)
	stopChannel := make(chan struct{}, 1)
	doneStopChannel := make(chan struct{}, 1)
	amqpBroker := &AMQPBroker{
		config:           cfg,
		connectionsMutex: sync.Mutex{},
		amqpErrorChannel: amqpErrorChannel,
		stopChannel:      stopChannel,
		doneStopChannel:  doneStopChannel,
		amqpProvider:     globalAmqpProvider,
	}

	if amqpBroker.amqpProvider == nil {
		amqpBroker.amqpProvider = provider.NewAmqpProvider(cfg.AMQP)
	}

	err := amqpBroker.amqpProvider.CreateConnectionPool()
	if err != nil {
		logger.ApplicationLogger.Error("failed to created connection pool, return", err)
		return nil, err
	}

	// Set up exchange, queue, and binding
	if err := amqpBroker.setupExchangeQueueBinding(); err != nil {
		logger.ApplicationLogger.Error("failed to created exchange queue binding, return", err)
		return nil, err
	}

	return amqpBroker, nil
}

func (b *AMQPBroker) processDelivery(ctx context.Context, amqpMsg amqp.Delivery, workerGroup workergroup.WorkerGroupInterface) error {
	defer b.processingWG.Done()
	if len(amqpMsg.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		err := amqpMsg.Nack(false, false)
		if err != nil {
			return err
		} // multiple, requeue
		return errors.ErrEmptyMessage // RabbitMQ down?
	}

	workerGroup.AssignJob(amqpMsg)
	return nil
}

// Publish sends a task to the AMQP broker.
// If the Signature contains RawArgs, it publishes only RawArgs as the message body,
// and encodes metadata in AMQP headers. Otherwise, it publishes the entire Signature as JSON.
func (b *AMQPBroker) Publish(ctx context.Context, signature *schema.Signature) error {
	if signature == nil {
		return errors.ErrEmptyMessage
	}

	// Always resolve a routing key before the message is built. Previously the
	// default was only applied on the non-delayed path, so a delayed task with
	// an empty RoutingKey was published with routing key "" and silently
	// dropped by the exchange.
	if signature.RoutingKey == "" {
		signature.RoutingKey = b.getRoutingKey()
	}

	body, headers, err := b.prepareMessageBodyAndHeaders(signature)
	if err != nil {
		return err
	}

	amqpPublishMessage := amqp.Publishing{
		ContentType:  "application/json",
		Priority:     signature.Priority,
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		MessageId:    signature.UUID,
		Timestamp:    time.Now().UTC(),
	}

	if delayMs := b.getTaskTTL(signature); delayMs > 0 {
		amqpPublishMessage.Expiration = fmt.Sprint(delayMs)
	}

	return b.amqpProvider.AmqpPublishWithConfirm(ctx, signature.RoutingKey, amqpPublishMessage, b.getExchangeName())
}

// prepareMessageBodyAndHeaders returns the AMQP message body and headers.
// If RawArgs is set on the Signature, RawArgs is used as the body and other metadata
// is encoded into headers. Otherwise, the entire Signature is marshaled into the body.
func (b *AMQPBroker) prepareMessageBodyAndHeaders(signature *schema.Signature) ([]byte, amqp.Table, error) {
	var body []byte
	var err error
	headers := amqp.Table{}

	if len(signature.RawArgs) > 0 && string(signature.RawArgs) != "null" {
		body = signature.RawArgs
		headers = buildHeadersFromSignature(signature)
	} else {
		body, err = json.Marshal(signature)
		if err != nil {
			return nil, nil, fmt.Errorf("JSON marshal error: %w", err)
		}
	}

	return body, headers, nil
}

// buildHeadersFromSignature converts task metadata fields from the Signature
// into AMQP headers. Used when publishing RawArgs-only messages.
func buildHeadersFromSignature(sig *schema.Signature) amqp.Table {
	headers := amqp.Table{
		"uuid":          sig.UUID,
		"task_name":     sig.Name,
		"retry_count":   sig.RetryCount,
		"retry_timeout": sig.RetryTimeout,
		"timeout":       sig.TaskTimeOut,
		"wait_time":     sig.WaitTime,
		"retries_done":  sig.RetriesDone,
		// routing_key and priority are read back by the consumer, so they have
		// to be written here too; without them a retried task lost its routing
		// key and every consumed task came back with priority 0.
		"routing_key":            sig.RoutingKey,
		"priority":               int(sig.Priority),
		"ignore_if_unregistered": sig.IgnoreWhenTaskNotRegistered,
	}

	if sig.CreatedAt != nil {
		headers["created_at"] = sig.CreatedAt.Unix()
	}
	if sig.ETA != nil {
		headers["eta"] = sig.ETA.Unix()
	}

	return headers
}

// setupExchangeQueueBinding sets up the exchange, queue, and binding
func (b *AMQPBroker) setupExchangeQueueBinding() error {
	amqpConn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider provider.AmqpProviderInterface, i interface{}) {
		if err := amqpProvider.ReleaseConnectionToPool(i); err != nil {
			logger.ApplicationLogger.Error("failed to release connection to pool", err)
		}
	}(b.amqpProvider, amqpConn)

	channel, _, err := b.amqpProvider.CreateAmqpChannel(amqpConn, false)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		if err := b.amqpProvider.CloseAmqpChannel(channel); err != nil {
			logger.ApplicationLogger.Error("failed to close amqp channel", err)
		}
	}(channel)

	// Declare the exchange
	err = b.amqpProvider.DeclareExchange(channel, b.getExchangeName(), b.getExchangeType())
	if err != nil {
		return err
	}

	declareQueueArgs := amqp.Table(b.config.AMQP.QueueDeclareArgs)

	// Declare the task queue
	err = b.amqpProvider.DeclareQueue(channel, b.getTaskQueue(), declareQueueArgs)
	if err != nil {
		return err
	}

	// Declare the delay queue
	declareQueueArgs = amqp.Table{
		// Exchange where to send messages after TTL expiration.
		"x-dead-letter-exchange": b.getDelayedQueueDLX(),
		// Routing key which use when resending expired messages.
		"x-dead-letter-routing-key": b.getRoutingKey(),
	}
	err = b.amqpProvider.DeclareQueue(channel, b.getDelayedQueue(), declareQueueArgs)
	if err != nil {
		return err
	}
	// Bind the queue to the exchange
	err = b.amqpProvider.QueueExchangeBind(channel, b.getTaskQueue(), b.config.AMQP.BindingKey, b.config.AMQP.Exchange)
	if err != nil {
		return err
	}

	// The task queue is also addressed by its own name (retries and the
	// dead-letter routing key both use it), so bind that too. Without this a
	// delayed message dead-lettered with routing key == queue name is dropped
	// whenever BindingKey differs from the queue name.
	if b.config.AMQP.BindingKey != b.getTaskQueue() {
		err = b.amqpProvider.QueueExchangeBind(channel, b.getTaskQueue(), b.getTaskQueue(), b.config.AMQP.Exchange)
		if err != nil {
			return err
		}
	}

	err = b.amqpProvider.QueueExchangeBind(channel, b.getDelayedQueue(), b.getDelayedQueue(), b.getDelayedQueueDLX())
	if err != nil {
		return err
	}

	if fq := b.getFailedQueue(); fq != "" {
		declareFailedQueueArgs := amqp.Table(b.config.AMQP.QueueDeclareArgs)
		// Bind Queue and Bind
		err = b.amqpProvider.DeclareQueue(channel, fq, declareFailedQueueArgs)
		if err != nil {
			return err
		}
		err = b.amqpProvider.QueueExchangeBind(channel, fq, fq, b.config.AMQP.Exchange)
		if err != nil {
			return err
		}
	}

	if tq := b.getTimeoutQueue(); tq != "" && tq != b.getFailedQueue() {
		declareTimeoutQueueArgs := amqp.Table(b.config.AMQP.QueueDeclareArgs)
		// Bind Timeout Queue and Bind
		err = b.amqpProvider.DeclareQueue(channel, tq, declareTimeoutQueueArgs)
		if err != nil {
			return err
		}
		err = b.amqpProvider.QueueExchangeBind(channel, tq, tq, b.config.AMQP.Exchange)
		if err != nil {
			return err
		}
	}

	return nil
}

// StopConsumer stops the AMQP consumer and waits for in-flight deliveries to be
// handed off to the worker group.
//
// It is safe to call when no consumer is running: the previous implementation
// blocked forever on an unbuffered stopChannel that nobody was reading.
func (b *AMQPBroker) StopConsumer() {
	if !b.consuming.Load() {
		logger.ApplicationLogger.Warning("stop requested but no consumer is running")
		return
	}

	select {
	case b.stopChannel <- struct{}{}:
	default:
		// A stop is already pending.
	}

	// Let deliveries already taken off the socket reach a worker, so nothing is
	// dropped on the floor during shutdown.
	if !waitTimeout(&b.processingWG, consumerStopTimeout) {
		logger.ApplicationLogger.Warning("timed out draining in-flight deliveries")
	}

	select {
	case <-b.doneStopChannel:
	case <-time.After(consumerStopTimeout):
		logger.ApplicationLogger.Warning("timed out waiting for consumer to stop")
	}
}

// waitTimeout waits for wg, returning false if the timeout elapsed first.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// consumerStopTimeout bounds how long StopConsumer waits for the consumer loop
// to acknowledge the stop request, so shutdown can never hang forever.
const consumerStopTimeout = 30 * time.Second

// StartConsumer initializes the AMQP consumer. It returns when the consumer is
// stopped (nil) or when the connection/channel is lost (non-nil error), leaving
// the caller free to reconnect.
func (b *AMQPBroker) StartConsumer(ctx context.Context, workerGroup workergroup.WorkerGroupInterface) error {
	if workerGroup == nil {
		return errors.ErrInvalidConfig
	}
	if !b.consuming.CompareAndSwap(false, true) {
		return fmt.Errorf("consumer is already running")
	}
	defer func() {
		b.consuming.Store(false)
		// Always signal completion so StopConsumer is released even when the
		// consumer exits on its own (connection loss, context cancellation).
		select {
		case b.doneStopChannel <- struct{}{}:
		default:
		}
	}()

	queueName := b.getTaskQueue()

	conn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider provider.AmqpProviderInterface, i interface{}) {
		if err := amqpProvider.ReleaseConnectionToPool(i); err != nil {
			logger.ApplicationLogger.Error("failed to release connection to pool", err)
		}
	}(b.amqpProvider, conn)

	// Create a channel
	channel, _, err := b.amqpProvider.CreateAmqpChannel(conn, false)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		if err := b.amqpProvider.CloseAmqpChannel(channel); err != nil {
			logger.ApplicationLogger.Error("failed to close amqp channel", err)
		}
	}(channel)

	// Channel QOS
	if err = b.amqpProvider.SetChannelQoS(channel, b.getQueuePrefetchCount()); err != nil {
		logger.ApplicationLogger.Error("failed to set channel qos, exit", err)
		return err
	}

	deliveries, err := b.amqpProvider.CreateConsumer(channel, queueName, workerGroup.GetWorkerGroupName())
	if err != nil {
		logger.ApplicationLogger.Error("failed to get deliveries, exit", err)
		return err
	}

	logger.ApplicationLogger.Info("[*] Waiting for messages. To exit press CTRL+C")

	errorsChan := make(chan error, 1)
	amqpErrorChannel := b.amqpErrorChannel
	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := channel.NotifyClose(make(chan *amqp.Error, 1))
	cancelled := channel.NotifyCancel(make(chan string, 1))

	for {
		select {
		case <-ctx.Done():
			logger.ApplicationLogger.Warning("context cancelled, consumer exiting")
			return nil
		case tag := <-cancelled:
			logger.ApplicationLogger.Error("RabbitMQ cancelled the consumer, exit")
			return fmt.Errorf("consumer cancelled by broker: %s", tag)
		case amqpErr := <-amqpErrorChannel:
			logger.ApplicationLogger.Error("error in consumer, exit", amqpErr)
			return amqpErr
		case err := <-errorsChan:
			logger.ApplicationLogger.Error("error in consumer, exit", err)
			return err
		case err := <-connClosed:
			logger.ApplicationLogger.Error("AMQP connection closed — RabbitMQ restarted:", err)
			return connectionError(err, "amqp connection closed")
		case err := <-chClosed:
			logger.ApplicationLogger.Error("AMQP channel closed — RabbitMQ restarted:", err)
			return connectionError(err, "amqp channel closed")
		case d, ok := <-deliveries:
			if !ok {
				logger.ApplicationLogger.Error("deliveries channel closed, exiting consumer loop")
				return errors.ErrDeliveriesChannelClosed
			}
			b.processingWG.Add(1)
			err := b.processDelivery(ctx, d, workerGroup)
			if err != nil {
				// A single bad delivery must not tear down the consumer;
				// returning here used to stop consumption completely the first
				// time an empty message arrived.
				logger.ApplicationLogger.Warning("delivery in error: ", err)
			}
		case <-b.stopChannel:
			logger.ApplicationLogger.Warning("stop request in consumer, exit")
			return nil
		}
	}
}

// connectionError normalises the nil *amqp.Error that NotifyClose delivers on a
// graceful close into a non-nil error, so callers can rely on `err != nil`
// meaning "the consumer must be restarted".
func connectionError(err *amqp.Error, msg string) error {
	if err == nil {
		return fmt.Errorf("%s", msg)
	}
	return err
}

func (b *AMQPBroker) getDelayedQueue() string {
	return b.config.AMQP.DelayedQueue
}

func (b *AMQPBroker) getFailedQueue() string {
	return b.config.AMQP.FailedQueue
}

func (b *AMQPBroker) getTimeoutQueue() string {
	return b.config.AMQP.TimeoutQueue
}

func (b *AMQPBroker) getQueuePrefetchCount() int {
	return b.config.AMQP.PrefetchCount
}

func (b *AMQPBroker) getDelayedQueueDLX() string {
	return b.config.AMQP.Exchange
}

func (b *AMQPBroker) getExchangeName() string {
	return b.config.AMQP.Exchange
}

func (b *AMQPBroker) getExchangeType() string {
	return b.config.AMQP.ExchangeType
}

func (b *AMQPBroker) getTaskQueue() string {
	return b.config.TaskQueueName
}

func (b *AMQPBroker) getTaskTTL(task *schema.Signature) int64 {
	// If ETA is not defined, no delay
	if task.ETA == nil {
		return 0
	}

	// If ETA is defined, check priority + TaskTimeOut + CreatedAt
	if task.Priority > 1 && task.TaskTimeOut != 0 && task.CreatedAt != nil {
		return 5 // send 5 milliseconds
	}

	// Otherwise calculate delay based on ETA
	now := time.Now().UTC()
	if task.ETA.After(now) {
		return int64(task.ETA.Sub(now) / time.Millisecond)
	}

	return 0
}
