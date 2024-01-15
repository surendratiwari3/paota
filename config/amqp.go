package config

// QueueBindingArgs arguments which are used when binding to the exchange
type QueueBindingArgs map[string]interface{}

// QueueDeclareArgs arguments which are used when declaring a queue
type QueueDeclareArgs map[string]interface{}

// AMQPConfig wraps RabbitMQ related configuration
type AMQPConfig struct {
	Exchange         string           `env:"EXCHANGE"`
	ExchangeType     string           `env:"EXCHANGE_TYPE"`
	QueueDeclareArgs QueueDeclareArgs `env:"QUEUE_DECLARE_ARGS"`
	QueueBindingArgs QueueBindingArgs `env:"QUEUE_BINDING_ARGS"`
	BindingKey       string           `env:"BINDING_KEY"`
	PrefetchCount    int              `env:"PREFETCH_COUNT"`
	AutoDelete       bool             `env:"AUTO_DELETE"`
	DelayedQueue     string           `env:"DELAYED_QUEUE"`
	QueueName        string           `env:"QUEUE_NAME"`
}
