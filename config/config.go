package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
)

var (
	applicationConfig Config
)

// Config holds all configuration for Paota
type Config struct {
	Broker         string         `env:"BROKER" envDefault:"amqp" validate:"required,oneof=amqp"` //allowed amqp
	Store          string         `env:"STORE"`
	TaskQueueName  string         `env:"QUEUE_NAME" envDefault:"paota_tasks" validate:"required"`
	StoreQueueName string         `env:"STORE_QUEUE_NAME"`
	AMQP           *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB        *MongoDBConfig `envPrefix:"MONGO_"`
}

func ReadFromEnv() error {
	envOpts := env.Options{
		Prefix: "PAOTA_",
	}
	err := env.ParseWithOptions(&applicationConfig, envOpts)
	if err != nil {
		return err
	}
	if err = ValidateConfig(applicationConfig); err != nil {
		return err
	}

	return nil
}

// ValidateConfig validates the configuration.
func ValidateConfig(cfg Config) error {
	// Use the validator package to perform validations
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
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
