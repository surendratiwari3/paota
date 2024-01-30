package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/logger"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/internal/schema"
	"github.com/surendratiwari3/paota/internal/schema/errors"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"sync"
	"time"
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
	if amqpConnection, ok := amqpConn.(*amqp.Connection); ok {
		return amqpConnection, nil
	}
	return nil, nil
}

// getRoutingKey gets the routing key from the signature
func (b *AMQPBroker) getRoutingKey() string {
	if b.config.AMQP.BindingKey == "" {
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
func NewAMQPBroker() (broker.Broker, error) {
	cfg := config.GetConfigProvider().GetConfig()
	amqpErrorChannel := make(chan *amqp.Error, 1)
	stopChannel := make(chan struct{})
	doneStopChannel := make(chan struct{})
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

func (b *AMQPBroker) processDelivery(ctx context.Context, d amqp.Delivery, workerGroup workergroup.WorkerGroupInterface) {
	// get worker from pool (blocks until one is available)
	workerGroup.GetWorker()
	// Consume the task inside a goroutine so multiple tasks
	// can be processed concurrently
	go func() {
		if err := b.taskProcessor(ctx, d, workerGroup); err != nil {
			logger.ApplicationLogger.Error("error in task processor, exit", err)
		}
		b.processingWG.Done()
		workerGroup.AddWorker()
	}()
}

// Publish sends a task to the AMQP broker
func (b *AMQPBroker) Publish(ctx context.Context, task *schema.Signature) error {
	// Convert task to JSON
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("JSON marshal error: %s", err)
	}

	// Get a connection from the pool
	conn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider provider.AmqpProviderInterface, i interface{}) {
		err := b.amqpProvider.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(b.amqpProvider, conn)

	// Create a channel
	channel, err := b.amqpProvider.CreateAmqpChannel(conn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := b.amqpProvider.CloseAmqpChannel(channel)
		if err != nil {
			//TODO:error handling
		}
	}(channel)

	amqpPublishMessage := amqp.Publishing{
		ContentType:  "application/json",
		Priority:     task.Priority,
		Body:         taskJSON,
		DeliveryMode: amqp.Persistent,
	}

	delayMs := b.getTaskTTL(task)
	// Set the routing key if not specified
	if delayMs > 0 {
		task.RoutingKey = b.getDelayedQueue()
		amqpPublishMessage.Expiration = fmt.Sprint(delayMs)
	} else if task.RoutingKey == "" {
		task.RoutingKey = b.getRoutingKey()
	}

	return b.amqpProvider.AmqpPublish(ctx, channel, task.RoutingKey, amqpPublishMessage, b.getExchangeName())
}

func (b *AMQPBroker) getTaskTTL(task *schema.Signature) int64 {
	var delayMs int64
	if task.ETA != nil {
		now := time.Now().UTC()
		if task.ETA.After(now) {
			delayMs = int64(task.ETA.Sub(now) / time.Millisecond)
		}
	}
	return delayMs
}

// setupExchangeQueueBinding sets up the exchange, queue, and binding
func (b *AMQPBroker) setupExchangeQueueBinding() error {
	amqpConn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider provider.AmqpProviderInterface, i interface{}) {
		err := b.amqpProvider.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(b.amqpProvider, amqpConn)

	channel, err := b.amqpProvider.CreateAmqpChannel(amqpConn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := b.amqpProvider.CloseAmqpChannel(channel)
		if err != nil {
			//TODO: error handling
		}
	}(channel)

	// Declare the exchange
	err = b.amqpProvider.DeclareExchange(channel, b.getExchangeName(), b.getExchangeType())
	if err != nil {
		return err
	}

	declareQueueArgs := amqp.Table{}

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

	err = b.amqpProvider.QueueExchangeBind(channel, b.getDelayedQueue(), b.getDelayedQueue(), b.getDelayedQueueDLX())
	if err != nil {
		return err
	}

	return nil
}

// StopConsumer stops the AMQP consumer
func (b *AMQPBroker) StopConsumer() {
	b.stopChannel <- struct{}{}
	<-b.doneStopChannel
}

// StartConsumer initializes the AMQP consumer
func (b *AMQPBroker) StartConsumer(ctx context.Context, workerGroup workergroup.WorkerGroupInterface) error {
	queueName := b.getTaskQueue()

	conn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpProvider provider.AmqpProviderInterface, i interface{}) {
		err := b.amqpProvider.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(b.amqpProvider, conn)

	// Create a channel
	channel, err := b.amqpProvider.CreateAmqpChannel(conn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := b.amqpProvider.CloseAmqpChannel(channel)
		if err != nil {
			logger.ApplicationLogger.Error("failed to start consumer, exit", err)
			//TODO:error handling
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
	for {
		select {
		case amqpErr := <-amqpErrorChannel:
			logger.ApplicationLogger.Error("error in consumer, exit", amqpErr)
			return amqpErr
		case err := <-errorsChan:
			logger.ApplicationLogger.Error("error in consumer, exit", err)
			return err
		case d := <-deliveries:
			b.processingWG.Add(1)
			b.processDelivery(ctx, d, workerGroup)
		case <-b.stopChannel:
			b.doneStopChannel <- struct{}{}
			logger.ApplicationLogger.Warning("stop request in consumer, exit")
			return nil
		}
	}
	b.processingWG.Wait()
	return nil
}

// consumerDeliveryHandler
func (b *AMQPBroker) taskProcessor(ctx context.Context, delivery amqp.Delivery, worker workergroup.WorkerGroupInterface) error {
	var multiple, requeue = false, false

	if len(delivery.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		delivery.Nack(multiple, requeue) // multiple, requeue
		return errors.ErrEmptyMessage    // RabbitMQ down?
	}

	signature, err := schema.BytesToSignature(delivery.Body)
	if err != nil {
		logger.ApplicationLogger.Error("decode error in message, return")
		delivery.Nack(multiple, requeue)
		return err
	}

	if err := worker.Processor(signature); err != nil {
		logger.ApplicationLogger.Errorf("Task failed to execute: %s", err.Error())

		if _, ok := err.(*errors.RetryError); !ok {
			// do nothing
		} else if signature.RetryCount < 1 {
			// do nothing
		} else if err := b.Publish(ctx, signature); err != nil {
			logger.ApplicationLogger.Error("failed to publish retry message")
		}

		err = delivery.Nack(false, false)
		return err
	}

	err = delivery.Ack(false)
	return err
}

func (b *AMQPBroker) getDelayedQueue() string {
	return b.config.AMQP.DelayedQueue
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
