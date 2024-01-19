package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"
	"sync"
)

// Broker represents an AMQP broker
type Broker struct {
	Cfg              *config.Config
	ConnectionPool   []*amqp.Connection
	connectionsMutex sync.Mutex
}

func (b *Broker) StartConsumer(consumerTag string, concurrency int) error {
	/*
		queueName := config.Conf.AMQP.QueueName
	*/
	return nil
}

func (b *Broker) StopConsumer() error {
	//TODO implement me
	return nil
}

func (b *Broker) Publish(ctx context.Context, task *task.Signature) error {

	rmqMsg, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("JSON marshal error: %s", err)
	}

	conn, err := b.GetConnection()
	if err != nil {
		return err
	}
	defer b.ReleaseConnection(conn)

	channel, err := conn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err = channel.ExchangeDeclare(
		b.Cfg.AMQP.Exchange,     // exchange name
		b.Cfg.AMQP.ExchangeType, // exchange type
		true,                    // durable
		false,                   // auto-deleted
		false,                   // internal
		false,                   // no-wait
		nil,                     // arguments
	); err != nil {
		return err
	}

	if task.RoutingKey == "" {
		task.RoutingKey = b.GetRoutingKey()
	}

	if err = channel.PublishWithContext(ctx,
		b.Cfg.AMQP.Exchange, // exchange
		task.RoutingKey,     // routing key
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Priority:     task.Priority,
			Body:         rmqMsg,
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return err
	}
	return nil
}

// NewBroker creates new Broker instance
// Connect opens a connection to RabbitMQ, declares an exchange, opens a channel,
// declares and binds the queue and enables publish notifications
func NewBroker(cnf *config.Config) (broker.Broker, error) {
	var err error
	amqpBroker := new(Broker)
	amqpBroker.Cfg = cnf
	amqpBroker.connectionsMutex = sync.Mutex{}

	if err = amqpBroker.initConnectionPool(10); err != nil {
		return nil, err
	}

	err = amqpBroker.setupExchangeQueueBinding()

	return amqpBroker, err
}

// InitConnectionPool initializes the connection pool
func (b *Broker) initConnectionPool(poolSize int) error {
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

func (b *Broker) createConnection() (*amqp.Connection, error) {
	conn, err := amqp.DialConfig(b.Cfg.AMQP.Url,
		amqp.Config{
			Heartbeat: 10, // Adjust heartbeat interval as needed
		},
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// GetConnection returns a connection from the pool
func (b *Broker) GetConnection() (*amqp.Connection, error) {
	b.connectionsMutex.Lock()
	defer b.connectionsMutex.Unlock()

	if b.ConnectionPool != nil && len(b.ConnectionPool) == 0 {
		return nil, fmt.Errorf("connection pool is empty")
	}

	conn := b.ConnectionPool[0]
	b.ConnectionPool = b.ConnectionPool[1:]

	return conn, nil
}

// ReleaseConnection releases a connection back to the pool
func (b *Broker) ReleaseConnection(conn *amqp.Connection) {
	b.connectionsMutex.Lock()
	defer b.connectionsMutex.Unlock()

	b.ConnectionPool = append(b.ConnectionPool, conn)
}

func (b *Broker) isDirectExchange() bool {
	return b.Cfg.AMQP != nil && b.Cfg.AMQP.ExchangeType == "direct"
}

// GetRoutingKey - get the routing key from signature
func (b *Broker) GetRoutingKey() string {
	if b.isDirectExchange() {
		return b.Cfg.AMQP.BindingKey
	}

	return b.Cfg.TaskQueueName
}

func (b *Broker) setupExchangeQueueBinding() error {
	amqpConn, err := b.GetConnection()
	if err != nil {
		return err
	}

	channel, err := amqpConn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	err = channel.ExchangeDeclare(
		b.Cfg.AMQP.Exchange,     // exchange name
		b.Cfg.AMQP.ExchangeType, // exchange type
		true,                    // durable
		false,                   // auto-deleted
		false,                   // internal
		false,                   // no-wait
		nil,                     // arguments
	)
	if err != nil {
		return err
	}

	_, err = channel.QueueDeclare(
		b.Cfg.TaskQueueName, // queue name
		true,                // durable
		false,               // delete when unused
		false,               // exclusive
		false,               // no-wait
		nil,                 // arguments
	)
	if err != nil {
		return err
	}

	err = channel.QueueBind(
		b.Cfg.TaskQueueName,   // queue name
		b.Cfg.AMQP.BindingKey, // routing key
		b.Cfg.AMQP.Exchange,   // exchange
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		return err
	}

	return nil
}
