package provider

import (
	"context"
	amqp "github.com/rabbitmq/amqp091-go"
)

type AmqpProviderInterface interface {
	AmqpPublish(ctx context.Context, channel *amqp.Channel, routingKey string, amqpMsg amqp.Publishing, exchangeName string) error
	CreateAmqpChannel(conn *amqp.Connection) (*amqp.Channel, error)
	CreateConsumer(channel *amqp.Channel, queueName, consumerTag string) (<-chan amqp.Delivery, error)
	DeclareQueue(channel *amqp.Channel, queueName string, declareQueueArgs amqp.Table) error
	DeclareExchange(channel *amqp.Channel, exchangeName string, exchangeType string) error
	QueueExchangeBind(channel *amqp.Channel, queueName string, routingKey string, exchangeName string) error
	SetChannelQoS(channel *amqp.Channel, prefetchCount int) error
}

type amqpProvider struct {
}

func (ap *amqpProvider) CreateAmqpChannel(conn *amqp.Connection) (*amqp.Channel, error) {
	return conn.Channel()
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

func NewAmqpProvider() AmqpProviderInterface {
	return &amqpProvider{}
}
