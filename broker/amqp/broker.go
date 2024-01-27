package amqp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/adapter"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/task"
	"sync"
)

// AMQPBroker represents an AMQP broker
type AMQPBroker struct {
	Config           *config.Config
	ackChannel       chan uint64
	amqpAdapter      adapter.Adapter
	connectionsMutex sync.Mutex
	processingWG     sync.WaitGroup
	amqpErrorChannel <-chan *amqp.Error
	stopChannel      chan struct{}
	doneStopChannel  chan struct{}
}

func (b *AMQPBroker) createAmqpChannel(conn *amqp.Connection) (*amqp.Channel, error) {
	return conn.Channel()
}

func (b *AMQPBroker) createConsumer(channel *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error) {
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

func (b *AMQPBroker) declareExchange(channel *amqp.Channel) error {
	return channel.ExchangeDeclare(
		b.Config.AMQP.Exchange,     // exchange name
		b.Config.AMQP.ExchangeType, // exchange type
		true,                       // durable
		false,                      // auto-deleted
		false,                      // internal
		false,                      // no-wait
		nil,                        // arguments
	)
}

func (b *AMQPBroker) declareQueue(channel *amqp.Channel) error {
	_, err := channel.QueueDeclare(
		b.Config.TaskQueueName, // queue name
		true,                   // durable
		false,                  // delete when unused
		false,                  // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	return err
}

func (b *AMQPBroker) getConnection() (*amqp.Connection, error) {
	amqpConn, err := b.amqpAdapter.GetConnectionFromPool()
	if err != nil {
		return nil, err
	}
	if amqpConnection, ok := amqpConn.(*amqp.Connection); ok {
		return amqpConnection, nil
	}
	return nil, nil
}

func (b *AMQPBroker) getQueueName() string {
	return config.GetConfigProvider().GetConfig().TaskQueueName
}

// getRoutingKey gets the routing key from the signature
func (b *AMQPBroker) getRoutingKey() string {
	if b.isDirectExchange() {
		return b.Config.AMQP.BindingKey
	}

	return b.Config.TaskQueueName
}

// isDirectExchange checks if the exchange type is direct
func (b *AMQPBroker) isDirectExchange() bool {
	return b.Config.AMQP != nil && b.Config.AMQP.ExchangeType == "direct"
}

// NewAMQPBroker creates a new instance of the AMQP broker
// It opens connections to RabbitMQ, declares an exchange, opens a channel,
// declares and binds the queue, and enables publish notifications
func NewAMQPBroker() (broker.Broker, error) {
	cfg := config.GetConfigProvider().GetConfig()
	amqpErrorChannel := make(chan *amqp.Error, 1)
	amqpBroker := &AMQPBroker{
		Config:           cfg,
		connectionsMutex: sync.Mutex{},
		amqpErrorChannel: amqpErrorChannel,
	}

	amqpBroker.amqpAdapter = adapter.NewAMQPAdapter()

	err := amqpBroker.amqpAdapter.CreateConnectionPool()
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

func (b *AMQPBroker) processDelivery(workers chan struct{}, d amqp.Delivery, registeredTasks *sync.Map) {
	// get worker from pool (blocks until one is available)
	<-workers
	// Consume the task inside a goroutine so multiple tasks
	// can be processed concurrently
	go func() {
		if err := b.taskProcessor(d, registeredTasks); err != nil {
			logger.ApplicationLogger.Error("error in task processor, exit", err)
			// TODO: error handling
		}
		err := d.Ack(false)
		if err != nil {
			logger.ApplicationLogger.Error("error while ack")
		}
		b.processingWG.Done()
		workers <- struct{}{}
	}()
}

func (b *AMQPBroker) queueExchangeBind(channel *amqp.Channel) error {
	// Bind the queue to the exchange
	return channel.QueueBind(
		b.Config.TaskQueueName,   // queue name
		b.Config.AMQP.BindingKey, // routing key
		b.Config.AMQP.Exchange,   // exchange
		false,                    // no-wait
		nil,                      // arguments
	)
}

// Publish sends a task to the AMQP broker
func (b *AMQPBroker) Publish(ctx context.Context, task *task.Signature) error {
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
	defer func(amqpAdapter adapter.Adapter, i interface{}) {
		err := b.amqpAdapter.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(b.amqpAdapter, conn)

	// Create a channel
	channel, err := b.createAmqpChannel(conn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			//TODO:error handling
		}
	}(channel)

	// Set the routing key if not specified
	if task.RoutingKey == "" {
		task.RoutingKey = b.getRoutingKey()
	}

	amqpPublishMessage := amqp.Publishing{
		ContentType:  "application/json",
		Priority:     task.Priority,
		Body:         taskJSON,
		DeliveryMode: amqp.Persistent,
	}

	return b.amqpPublish(ctx, channel, task.RoutingKey, amqpPublishMessage)

}

func (b *AMQPBroker) amqpPublish(ctx context.Context, channel *amqp.Channel, routingKey string, amqpMsg amqp.Publishing) error {
	// Publish the message to the exchange
	err := channel.PublishWithContext(ctx,
		b.Config.AMQP.Exchange, // exchange
		routingKey,             // routing key
		false,                  // mandatory
		false,                  // immediate
		amqpMsg,
	)
	return err
}

func (b *AMQPBroker) setChannelQoS(channel *amqp.Channel) error {
	return channel.Qos(
		config.GetConfigProvider().GetConfig().AMQP.PrefetchCount,
		0,     // prefetch size
		false, // global
	)
}

// setupExchangeQueueBinding sets up the exchange, queue, and binding
func (b *AMQPBroker) setupExchangeQueueBinding() error {
	amqpConn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpAdapter adapter.Adapter, i interface{}) {
		err := amqpAdapter.ReleaseConnectionToPool(i)
		if err != nil {
			logger.ApplicationLogger.Error("release connection to pool failed", err)
			//TODO:error handling
		}
	}(b.amqpAdapter, amqpConn)

	channel, err := b.createAmqpChannel(amqpConn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			//TODO: error handling
		}
	}(channel)

	// Declare the exchange
	err = b.declareExchange(channel)
	if err != nil {
		return err
	}

	// Declare the queue
	err = b.declareQueue(channel)
	if err != nil {
		return err
	}

	// Bind the queue to the exchange
	err = b.queueExchangeBind(channel)
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
func (b *AMQPBroker) StartConsumer(consumerTag string, workers chan struct{}, registeredTasks *sync.Map) error {
	queueName := b.getQueueName()

	conn, err := b.getConnection()
	if err != nil {
		return err
	}
	defer func(amqpAdapter adapter.Adapter, i interface{}) {
		err := amqpAdapter.ReleaseConnectionToPool(i)
		if err != nil {
			//TODO:error handling
		}
	}(b.amqpAdapter, conn)

	// Create a channel
	channel, err := b.createAmqpChannel(conn)
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			logger.ApplicationLogger.Error("failed to start consumer, exit", err)
			//TODO:error handling
		}
	}(channel)

	// Channel QOS
	if err = b.setChannelQoS(channel); err != nil {
		logger.ApplicationLogger.Error("failed to set channel qos, exit", err)
		return err
	}

	deliveries, err := b.createConsumer(channel, queueName, consumerTag)
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
			b.processDelivery(workers, d, registeredTasks)
		case <-b.stopChannel:
			logger.ApplicationLogger.Warning("stop request in consumer, exit")
			return nil
		}
	}
	b.processingWG.Wait()
	return nil
}

// consumerDeliveryHandler
func (b *AMQPBroker) taskProcessor(delivery amqp.Delivery, registeredTask *sync.Map) error {
	if len(delivery.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		delivery.Nack(true, false)    // multiple, requeue
		return errors.ErrEmptyMessage // RabbitMQ down?
	}

	var multiple, requeue = false, false

	// Unmarshal message body into signature struct
	signature := new(task.Signature)
	decoder := json.NewDecoder(bytes.NewReader(delivery.Body))
	decoder.UseNumber()
	if err := decoder.Decode(signature); err != nil {
		logger.ApplicationLogger.Error("decode error in message, return")
		delivery.Nack(multiple, requeue)
		return err
	}

	// Check if the task is registered
	if taskFunc, ok := registeredTask.Load(signature.Name); ok {
		if fn, ok := taskFunc.(func(*task.Signature) error); ok {
			return fn(signature)
		}
		// Handle the case where the value in registeredTask is not a function
		return errors.ErrTaskMustBeFunc
	}

	return errors.ErrTaskNotRegistered
}
