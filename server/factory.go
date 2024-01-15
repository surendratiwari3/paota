package server

import (
	"fmt"
	"github.com/surendratiwari3/paota/backend"
	"github.com/surendratiwari3/paota/backend/null"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	"strings"
)

// BrokerFactory creates a new object of broker.Broker
func BrokerFactory(cnf *config.Config) (broker.Broker, error) {
	if strings.HasPrefix(cnf.Broker, "amqp://") {
		return amqp.New(cnf), nil
	}

	if strings.HasPrefix(cnf.Broker, "amqps://") {
		return amqp.New(cnf), nil
	}

	return nil, fmt.Errorf("Factory failed with broker URL: %v", cnf.Broker)
}

// BackendFactory creates a new object of backends.Interface
func BackendFactory(cnf *config.Config) (backend.Backend, error) {

	if strings.HasPrefix(cnf.StoreBackend, "null") {
		return null.New(), nil
	}
	return nil, fmt.Errorf("Factory failed with result backend: %v", cnf.StoreBackend)
}
