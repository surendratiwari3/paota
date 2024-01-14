package amqp

import (
	"context"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"
)

// Broker represents an AMQP broker
type Broker struct {
}

func (b Broker) StartConsuming(consumerTag string, concurrency int) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (b Broker) StopConsuming() {
	//TODO implement me
	panic("implement me")
}

func (b Broker) Enqueue(ctx context.Context, task *task.Signature) error {
	//TODO implement me
	panic("implement me")
}

// New creates new Broker instance
func New(cnf *config.Config) broker.Broker {
	return &Broker{}
}
