package errors

import (
	"errors"
)

var (
	ErrUnregisteredTask        = errors.New("unregistered task")
	ErrTaskFunctionNotFound    = errors.New("task function not found")
	ErrFailedToStartWorkerPool = errors.New("failed to start the worker pool")
	ErrInvalidWorkerContext    = errors.New("work: Context needs to be a struct type")
)
