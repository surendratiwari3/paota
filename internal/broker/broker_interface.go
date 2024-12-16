package broker

import (
	"context"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/schema"
)

// Broker - a common interface for all brokers
type Broker interface {
	StartConsumer(ctx context.Context, groupInterface workergroup.WorkerGroupInterface) error
	StopConsumer()
	Publish(ctx context.Context, task *schema.Signature) error
	BrokerType() string 
}
