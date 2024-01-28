package broker

import (
	"context"
	"github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/workergroup"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsumer(worker *workergroup.WorkerGroup) error
	StopConsumer()
	Publish(ctx context.Context, task *task.Signature) error
}
