package broker

import (
	"context"
	"github.com/surendratiwari3/paota/task"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsumer(consumerTag string, concurrency int) error
	StopConsumer() error
	Publish(ctx context.Context, task *task.Signature) error
}
