package factory

import (
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	amqpBroker "github.com/surendratiwari3/paota/internal/broker/amqp"
	"github.com/surendratiwari3/paota/internal/broker/redis"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/internal/task/memory"
	"github.com/surendratiwari3/paota/logger"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

type IFactory interface {
	CreateBroker(configProvider config.ConfigProvider) (broker.Broker, error)
	CreateStore(configProvider config.ConfigProvider) error
	CreateTaskRegistrar(brk broker.Broker, configProvider config.ConfigProvider) task.TaskRegistrarInterface
}

type Factory struct{}

// NewAMQPBroker creates a new instance of AMQPBroker
func (bf *Factory) NewAMQPBroker(configProvider config.ConfigProvider) (broker.Broker, error) {
	return amqpBroker.NewAMQPBroker(configProvider)
}

func (bf *Factory) NewRedisBroker(configProvider config.ConfigProvider) (broker.Broker, error) {
	redisProvider := provider.NewRedisProvider(configProvider.GetConfig().Redis)
	return redis.NewRedisBroker(redisProvider, configProvider.GetConfig())
}

// CreateBroker creates a new object of broker.Broker
func (bf *Factory) CreateBroker(configProvider config.ConfigProvider) (broker.Broker, error) {
	brokerType := configProvider.GetConfig().Broker
	switch brokerType {
	case "amqp":
		return bf.NewAMQPBroker(configProvider)
	case "redis":
		return bf.NewRedisBroker(configProvider)
	default:
		logger.ApplicationLogger.Error("unsupported broker")
		return nil, appErrors.ErrUnsupportedBroker
	}
}

// CreateStore creates a new object of store.Interface
func (bf *Factory) CreateStore(configProvider config.ConfigProvider) error {
	storeBackend := configProvider.GetConfig().Store
	switch storeBackend {
	case "":
		return nil
	default:
		return appErrors.ErrUnsupportedStore
	}
}

func (bf *Factory) CreateTaskRegistrar(brk broker.Broker, configProvider config.ConfigProvider) task.TaskRegistrarInterface {
	return memory.NewDefaultTaskRegistrar(brk, configProvider)
}
