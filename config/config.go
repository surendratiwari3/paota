// config.go

package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
)

// ConfigProvider is an interface for retrieving configuration.
type ConfigProvider interface {
	ReadFromEnv() error
	ValidateConfig(cfg Config) error
	GetConfig() *Config
}

// Config holds all configuration for Paota
type Config struct {
	Broker         string         `env:"BROKER" envDefault:"amqp" validate:"required,oneof=amqp"` //allowed amqp
	Store          string         `env:"STORE"`
	TaskQueueName  string         `env:"QUEUE_NAME" envDefault:"paota_tasks" validate:"required"`
	StoreQueueName string         `env:"STORE_QUEUE_NAME"`
	AMQP           *AMQPConfig    `envPrefix:"AMQP_"`
	MongoDB        *MongoDBConfig `envPrefix:"MONGO_"`
}

type configProvider struct {
	applicationConfig Config
}

var (
	globalConfigProvider ConfigProvider
)

// NewConfigProvider creates a new instance of ConfigProvider.
func NewConfigProvider() ConfigProvider {
	return &configProvider{}
}

func (cp *configProvider) ReadFromEnv() error {
	envOpts := env.Options{
		Prefix: "PAOTA_",
	}
	err := env.ParseWithOptions(&cp.applicationConfig, envOpts)
	if err != nil {
		return err
	}
	if err = cp.ValidateConfig(cp.applicationConfig); err != nil {
		return err
	}

	return nil
}

func (cp *configProvider) ValidateConfig(cfg Config) error {
	// Use the validator package to perform validations
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return err
	}

	return nil
}

func (cp *configProvider) GetConfig() *Config {
	emptyStruct := Config{}
	if cp.applicationConfig == emptyStruct {
		return nil
	}
	return &cp.applicationConfig
}

// SetConfigProvider sets the global configuration provider.
// This is useful for injecting custom configurations during tests.
func SetConfigProvider(provider ConfigProvider) {
	globalConfigProvider = provider
}

// GetConfigProvider returns the global configuration provider.
func GetConfigProvider() ConfigProvider {
	if globalConfigProvider == nil {
		globalConfigProvider = NewConfigProvider()
	}
	return globalConfigProvider
}
