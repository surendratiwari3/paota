package backend

import "github.com/surendratiwari3/paota/task"

// Backend - a common interface for all result backends
type Backend interface {
	// InsertTask Insert task state to result backend
	InsertTask(signature task.Signature) error
	// DeleteTask Delete task state from result backend
	DeleteTask(taskId string) error
	// GetTask Get task state from result backend
	GetTask(taskId string) ([]task.Signature, error)
	// GetTaskState Get task from result backend
	GetTaskState(taskId string) (string, error)
}
