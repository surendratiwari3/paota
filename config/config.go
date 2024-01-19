package config

// Config holds all configuration for Paota
type Config struct {
	Broker         string         `env:"BROKER"` //allowed amqp
	StoreBroker    string         `env:"STORE_BROKER"`
	Store          string         `env:"STORE"`
	TaskQueueName  string         `env:"QUEUE_NAME"`
	StoreQueueName string         `env:"STORE_QUEUE_NAME"`
	AMQP           *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB        *MongoDBConfig `envPrefix:"MONGO_"`
}

type Consumer struct {
	Tag         string `env:"CONSUMER_TAG"`
	Concurrency int    `env:"CONCURRENCY"`
}

var Conf = Config{}
