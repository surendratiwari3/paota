package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"
)

// AMQPBroker represents an AMQP broker
type AMQPBroker struct {
	Config           *config.Config
	ConnectionPool   []*amqp.Connection
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
	defer b.releaseConnection(conn)

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
func NewAMQPBroker(cfg *config.Config) (broker.Broker, error) {
	amqpBroker := &AMQPBroker{
		Config:           cfg,
		connectionsMutex: sync.Mutex{},
	}

	// Initialize the connection pool
	if err := amqpBroker.initConnectionPool(); err != nil {
		return nil, err
	}

	// Set up exchange, queue, and binding
	if err := amqpBroker.setupExchangeQueueBinding(); err != nil {
		return nil, err
	}

	return amqpBroker, nil
}

// initConnectionPool initializes the connection pool
func (b *AMQPBroker) initConnectionPool() error {
	poolSize := b.Config.AMQP.ConnectionPoolSize
	if poolSize < 2 {
		return nil
	}
	connPool := make([]*amqp.Connection, poolSize)

	for i := 0; i < poolSize; i++ {
		conn, err := b.createConnection()
		if err != nil {
			return err
		}

		connPool[i] = conn
	}
	b.ConnectionPool = connPool

	return nil
}

// createConnection creates a new AMQP connection
func (b *AMQPBroker) createConnection() (*amqp.Connection, error) {
	conn, err := amqp.DialConfig(b.Config.AMQP.Url,
		amqp.Config{
			Heartbeat: time.Duration(b.Config.AMQP.HeartBeatInterval), // Adjust heartbeat interval as needed
		},
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// getConnection returns a connection from the pool
func (b *AMQPBroker) getConnection() (*amqp.Connection, error) {
	b.connectionsMutex.Lock()
	defer b.connectionsMutex.Unlock()

	if b.ConnectionPool != nil && len(b.ConnectionPool) == 0 {
		return nil, fmt.Errorf("connection pool is empty")
	}

	conn := b.ConnectionPool[0]
	b.ConnectionPool = b.ConnectionPool[1:]

	return conn, nil
}

// releaseConnection releases a connection back to the pool
func (b *AMQPBroker) releaseConnection(conn *amqp.Connection) {
	b.connectionsMutex.Lock()
	defer b.connectionsMutex.Unlock()

	b.ConnectionPool = append(b.ConnectionPool, conn)
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
	if err != nil {
		return err
	}

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
