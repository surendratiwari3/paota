package config

import (
	"github.com/caarlos0/env/v10"
)

var (
	applicationConfig Config
)

// Config holds all configuration for Paota
type Config struct {
	Broker         string         `env:"BROKER" envDefault:"amqp"` //allowed amqp
	Store          string         `env:"STORE"`
	TaskQueueName  string         `env:"QUEUE_NAME" envDefault:"paota_tasks"`
	StoreQueueName string         `env:"STORE_QUEUE_NAME"`
	AMQP           *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB        *MongoDBConfig `envPrefix:"MONGO_"`
}

type Consumer struct {
	Tag         string `env:"CONSUMER_TAG" envDefault:"paota_worker"`
	Concurrency int    `env:"CONCURRENCY" envDefault:"10"`
}

func ReadFromEnv() error {
	envOpts := env.Options{
		Prefix: "PAOTA_",
	}
	err := env.ParseWithOptions(&applicationConfig, envOpts)
	if err != nil {
		return err
	}
	return nil
}

func GetConfig() *Config {
	emptyStruct := Config{}
	if applicationConfig == emptyStruct {
		return nil
	}
	return &applicationConfig
}
