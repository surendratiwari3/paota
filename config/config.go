package config

// Config holds all configuration for Paota
type Config struct {
	Broker       string         `env:"BROKER"`
	StoreBackend string         `env:"STORE_BACKEND"`
	AMQP         *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB      *MongoDBConfig `envPrefix:"MONGO_"`
}
