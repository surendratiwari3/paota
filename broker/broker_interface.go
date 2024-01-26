package broker

import (
	"context"
	"github.com/surendratiwari3/paota/task"
	"sync"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsumer(consumerTag string, workers chan struct{}, registeredTasks *sync.Map) error
	StopConsumer()
	Publish(ctx context.Context, task *task.Signature) error
}
