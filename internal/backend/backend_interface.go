package backend

import (
	"github.com/surendratiwari3/paota/schema"
)

// Backend - a common interface for all result Store
type Backend interface {
	// StoreTask Insert task state to result backend
	StoreTask(signature schema.Signature) error
	// DeleteTask Delete task state from result backend
	DeleteTask(taskId string) error
	// GetTask Get task state from result backend
	GetTask(taskId string) ([]schema.Signature, error)
	// GetTaskState Get task from result backend
	GetTaskState(taskId string) (string, error)
}
