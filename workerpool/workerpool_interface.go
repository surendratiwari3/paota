package workerpool

import (
	"context"
	"github.com/surendratiwari3/paota/internal/backend"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/schema"
)

type Pool interface {
	GetBackend() backend.Backend
	GetBroker() broker.Broker
	IsTaskRegistered(name string) bool
	RegisterTasks(namedTaskFuncs map[string]interface{}) error
	SetBackend(backend backend.Backend)
	SetBroker(broker broker.Broker)
	Start() error
	Stop()
	SendTaskWithContext(ctx context.Context, signature *schema.Signature) (*schema.State, error)
}
