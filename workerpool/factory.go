package workerpool

import (
	"github.com/surendratiwari3/paota/broker"
	amqpBroker "github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/store"
)

// CreateBroker creates a new object of broker.Broker
func CreateBroker() (broker.Broker, error) {
	brokerType := config.GetConfigProvider().GetConfig().Broker
	switch brokerType {
	case "amqp":
		return amqpBroker.NewAMQPBroker()
	default:
		logger.ApplicationLogger.Error("unsupported broker")
		return nil, errors.ErrUnsupportedBroker
	}
}

// CreateStore creates a new object of store.Interface
func CreateStore() (store.Backend, error) {
	storeBackend := config.GetConfigProvider().GetConfig().Store
	switch storeBackend {
	case "":
		return nil, nil
	default:
		return nil, errors.ErrUnsupportedStore
	}
}
