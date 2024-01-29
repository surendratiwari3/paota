package workerpool

import (
	"github.com/surendratiwari3/paota/broker"
	amqpBroker "github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	appErrors "github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/task"
)

type IFactory interface {
	CreateBroker() (broker.Broker, error)
	CreateStore() error
	CreateTaskRegistrar() task.TaskRegistrarInterface
}

type Factory struct{}

// NewAMQPBroker creates a new instance of AMQPBroker
func (bf *Factory) NewAMQPBroker() (broker.Broker, error) {
	return amqpBroker.NewAMQPBroker()
}

// CreateBroker creates a new object of broker.Broker
func (bf *Factory) CreateBroker() (broker.Broker, error) {
	brokerType := config.GetConfigProvider().GetConfig().Broker
	switch brokerType {
	case "amqp":
		return bf.NewAMQPBroker()
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

func (bf *Factory) CreateTaskRegistrar() task.TaskRegistrarInterface {
	return task.NewDefaultTaskRegistrar()
}
