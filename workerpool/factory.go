package workerpool

import (
	"github.com/surendratiwari3/paota/broker"
	amqpBroker "github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"
)

// CreateBroker creates a new object of broker.Broker
func CreateBroker(taskChannel chan task.Job) (broker.Broker, error) {
	brokerType := config.GetConfigProvider().GetConfig().Broker
	switch brokerType {
	case "amqp":
		return amqpBroker.NewAMQPBroker(taskChannel)
	default:
		return nil, errors.ErrUnsupportedBroker
	}
}

// CreateStore creates a new object of store.Interface
func CreateStore() (store.Backend, error) {
	storeBackend := config.GetConfigProvider().GetConfig().Store
	switch storeBackend {
	default:
		return nil, errors.ErrUnsupportedStore
	}
}
