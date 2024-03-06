package workerpool

import (
	"context"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/store"
	"github.com/surendratiwari3/paota/schema"
)

type Pool interface {
	GetBackend() store.Backend
	GetBroker() broker.Broker
	IsTaskRegistered(name string) bool
	RegisterTasks(namedTaskFuncs map[string]interface{}) error
	SetBackend(backend store.Backend)
	SetBroker(broker broker.Broker)
	Start() error
	Stop()
	SendTaskWithContext(ctx context.Context, signature *schema.Signature) (*schema.State, error)
}
