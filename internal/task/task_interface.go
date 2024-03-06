package task

import (
	"context"
	"github.com/surendratiwari3/paota/schema"
)

type TaskRegistrarInterface interface {
	IsTaskRegistered(name string) bool
	GetRegisteredTask(name string) (interface{}, error)
	RegisterTasks(namedTaskFuncs map[string]interface{}) error
	GetRegisteredTaskCount() uint
	Processor(interface{}) error
	SendTask(signature *schema.Signature) error
	SendTaskWithContext(ctx context.Context, signature *schema.Signature) error
}
