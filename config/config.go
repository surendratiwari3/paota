package config

// Config holds all configuration for Paota
type Config struct {
	Broker       string         `env:"BROKER"`
	StoreBackend string         `env:"STORE_BACKEND"`
	AMQP         *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB      *MongoDBConfig `envPrefix:"MONGO_"`
}

type Consumer struct {
	Tag         string `env:"CONSUMER_TAG"`
	Concurrency int    `env:"Concurrency"`
}

var Conf = Config{}
