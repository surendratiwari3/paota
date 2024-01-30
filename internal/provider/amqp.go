package provider

import (
	"context"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/schema/errors"
	"sync"
	"time"
)

type AmqpProviderInterface interface {
	AmqpPublish(ctx context.Context, channel *amqp.Channel, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error
	CreateAmqpChannel(conn *amqp.Connection) (*amqp.Channel, error)
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

func (ap *amqpProvider) CreateAmqpChannel(conn *amqp.Connection) (*amqp.Channel, error) {
	return conn.Channel()
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
}

// CreateConnectionPool initializes the connection pool
func (ap *amqpProvider) CreateConnectionPool() error {
	poolSize := ap.amqpConf.ConnectionPoolSize
	if poolSize < 2 {
		return errors.ErrInvalidConfig
	}
	connPool := make([]*amqp.Connection, poolSize)

	for i := 0; i < poolSize; i++ {
		conn, err := ap.CreateConnection()
		if err != nil {
			return err
		}

		connPool[i] = conn
	}
	ap.ConnectionPool = connPool
	return nil
}

// ReleaseConnectionToPool releases a connection back to the pool
func (ap *amqpProvider) ReleaseConnectionToPool(conn interface{}) error {

	if amqpConnection, ok := conn.(*amqp.Connection); ok {

		ap.connectionsMutex.Lock()
		defer ap.connectionsMutex.Unlock()

		ap.ConnectionPool = append(ap.ConnectionPool, amqpConnection)
	}
	return nil
}

// GetConnectionFromPool returns a connection from the pool
func (ap *amqpProvider) GetConnectionFromPool() (interface{}, error) {
	ap.connectionsMutex.Lock()
	defer ap.connectionsMutex.Unlock()

	if ap.ConnectionPool == nil || len(ap.ConnectionPool) == 0 {
		return nil, errors.ErrConnectionPoolEmpty
	}

	conn := ap.ConnectionPool[0]
	ap.ConnectionPool = ap.ConnectionPool[1:]

	return conn, nil
}
