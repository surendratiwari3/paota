package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/adapter"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"
	"sync"
)

// AMQPBroker represents an AMQP broker
type AMQPBroker struct {
	Config           *config.Config
	amqpAdapter      adapter.Adapter
	connectionsMutex sync.Mutex
}

// StartConsumer initializes the AMQP consumer
func (b *AMQPBroker) StartConsumer(consumerTag string, concurrency int) error {
	// TODO: Implement consumer setup (if needed)
	/*
		queueName := config.Conf.AMQP.QueueName
	*/
	return nil
}

// StopConsumer stops the AMQP consumer
func (b *AMQPBroker) StopConsumer() error {
	// TODO: Implement consumer stop logic
	return nil
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
	defer b.amqpAdapter.ReleaseConnectionToPool(conn)

	// Create a channel
	channel, err := conn.Channel()
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			//TODO:
		}
	}(channel)

	// Set the routing key if not specified
	if task.RoutingKey == "" {
		task.RoutingKey = b.getRoutingKey()
	}

	// Publish the message to the exchange
	if err = channel.PublishWithContext(ctx,
		b.Config.AMQP.Exchange, // exchange
		task.RoutingKey,        // routing key
		false,                  // mandatory
		false,                  // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Priority:     task.Priority,
			Body:         taskJSON,
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return err
	}

	return nil
}

// NewAMQPBroker creates a new instance of the AMQP broker
// It opens connections to RabbitMQ, declares an exchange, opens a channel,
// declares and binds the queue, and enables publish notifications
func NewAMQPBroker() (broker.Broker, error) {
	cfg := config.GetConfigProvider().GetConfig()
	amqpBroker := &AMQPBroker{
		Config:           cfg,
		connectionsMutex: sync.Mutex{},
	}

	amqpBroker.amqpAdapter = adapter.NewAMQPAdapter()

	err := amqpBroker.amqpAdapter.CreateConnectionPool()
	if err != nil {
		return nil, err
	}

	// Set up exchange, queue, and binding
	if err := amqpBroker.setupExchangeQueueBinding(); err != nil {
		return nil, err
	}

	return amqpBroker, nil
}

// isDirectExchange checks if the exchange type is direct
func (b *AMQPBroker) isDirectExchange() bool {
	return b.Config.AMQP != nil && b.Config.AMQP.ExchangeType == "direct"
}

// getRoutingKey gets the routing key from the signature
func (b *AMQPBroker) getRoutingKey() string {
	if b.isDirectExchange() {
		return b.Config.AMQP.BindingKey
	}

	return b.Config.TaskQueueName
}

// setupExchangeQueueBinding sets up the exchange, queue, and binding
func (b *AMQPBroker) setupExchangeQueueBinding() error {
	amqpConn, err := b.getConnection()
	channel, err := amqpConn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	// Declare the exchange
	err = channel.ExchangeDeclare(
		b.Config.AMQP.Exchange,     // exchange name
		b.Config.AMQP.ExchangeType, // exchange type
		true,                       // durable
		false,                      // auto-deleted
		false,                      // internal
		false,                      // no-wait
		nil,                        // arguments
	)
	if err != nil {
		return err
	}

	// Declare the queue
	_, err = channel.QueueDeclare(
		b.Config.TaskQueueName, // queue name
		true,                   // durable
		false,                  // delete when unused
		false,                  // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	if err != nil {
		return err
	}

	// Bind the queue to the exchange
	err = channel.QueueBind(
		b.Config.TaskQueueName,   // queue name
		b.Config.AMQP.BindingKey, // routing key
		b.Config.AMQP.Exchange,   // exchange
		false,                    // no-wait
		nil,                      // arguments
	)
	if err != nil {
		return err
	}

	return nil
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
