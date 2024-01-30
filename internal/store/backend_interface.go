package store

import (
	"github.com/surendratiwari3/paota/internal/schema"
)

// Backend - a common interface for all result Store
type Backend interface {
	// InsertTask Insert task state to result backend
	InsertTask(signature schema.Signature) error
	// DeleteTask Delete task state from result backend
	DeleteTask(taskId string) error
	// GetTask Get task state from result backend
	GetTask(taskId string) ([]schema.Signature, error)
	// GetTaskState Get task from result backend
	GetTaskState(taskId string) (string, error)
}
