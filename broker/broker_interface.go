package broker

import (
	"context"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workergroup"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsumer(ctx context.Context, groupInterface workergroup.WorkerGroupInterface) error
	StopConsumer()
	Publish(ctx context.Context, task *schema.Signature) error
}
