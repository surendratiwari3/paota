package broker

import (
	"context"
	"github.com/surendratiwari3/paota/task"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsuming(consumerTag string, concurrency int) error
	StopConsuming() error
	Enqueue(ctx context.Context, task *task.Signature) error
}
