package factory

import (
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	amqpBroker "github.com/surendratiwari3/paota/internal/broker/amqp"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/internal/task/memory"
	"github.com/surendratiwari3/paota/logger"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

type IFactory interface {
	CreateBroker(broker string) (broker.Broker, error)
	CreateStore() error
	CreateTaskRegistrar(brk broker.Broker, brkFailover broker.Broker) task.TaskRegistrarInterface
}

type Factory struct{}

// NewAMQPBroker creates a new instance of AMQPBroker
func (bf *Factory) NewAMQPBroker(brokerType string) (broker.Broker, error) {
	return amqpBroker.NewAMQPBroker(brokerType)
}

// CreateBroker creates a new object of broker.Broker
func (bf *Factory) CreateBroker(broker string) (broker.Broker, error) {
	brokerType := config.GetConfigProvider().GetConfig().Broker
	switch brokerType {
	case "amqp":
		return bf.NewAMQPBroker(broker)
	default:
		logger.ApplicationLogger.Error("unsupported broker")
		return nil, appErrors.ErrUnsupportedBroker
	}
}

// CreateStore creates a new object of store.Interface
func (bf *Factory) CreateStore() error {
	storeBackend := config.GetConfigProvider().GetConfig().Store
	switch storeBackend {
	case "":
		return nil
	default:
		return appErrors.ErrUnsupportedStore
	}
}

func (bf *Factory) CreateTaskRegistrar(brk broker.Broker, brkFailover broker.Broker) task.TaskRegistrarInterface {
	return memory.NewDefaultTaskRegistrar(brk, brkFailover)
}
