package config

// QueueBindingArgs arguments which are used when binding to the exchange
type QueueBindingArgs map[string]interface{}

// QueueDeclareArgs arguments which are used when declaring a queue
type QueueDeclareArgs map[string]interface{}

// AMQPConfig wraps RabbitMQ related configuration
type AMQPConfig struct {
	Url                string           `env:"URL" validate:"required"`
	Exchange           string           `env:"EXCHANGE" envDefault:"paota_task_exchange"`
	ExchangeType       string           `env:"EXCHANGE_TYPE" envDefault:"direct"`
	QueueDeclareArgs   QueueDeclareArgs `env:"QUEUE_DECLARE_ARGS"`
	QueueBindingArgs   QueueBindingArgs `env:"QUEUE_BINDING_ARGS"`
	BindingKey         string           `env:"BINDING_KEY" envDefault:"paota_task_binding_key"`
	PrefetchCount      int              `env:"PREFETCH_COUNT" validate:"required"`
	AutoDelete         bool             `env:"AUTO_DELETE"`
	DelayedQueue       string           `env:"DELAYED_QUEUE" envDefault:"paota_task_delayed_queue"`
	FailedQueue        string           `env:"FAILED_QUEUE" envDefault:"paota_task_failed_queue"`
	TimeoutQueue       string           `env:"TIMEOUT_QUEUE" envDefault:"paota_task_failed_queue"`
	ConnectionPoolSize int              `env:"CONNECTION_POOL_SIZE" envDefault:"5"`
	HeartBeatInterval  int              `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
}
