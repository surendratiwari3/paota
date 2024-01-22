package worker

import (
	"github.com/surendratiwari3/paota/broker"
	amqpBroker "github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/store"
)

// CreateBroker creates a new object of broker.Broker
func CreateBroker(cnf *config.Config) (broker.Broker, error) {
	switch cnf.Broker {
	case "amqp":
		return amqpBroker.NewAMQPBroker()
	default:
		return nil, errors.ErrUnsupportedBroker
	}
}

// CreateStore creates a new object of store.Interface
func CreateStore(cnf *config.Config) (store.Backend, error) {
	switch cnf.Store {
	default:
		return nil, errors.ErrUnsupportedStore
	}
}
