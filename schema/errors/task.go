package errors

import "errors"

var (
	// ErrTaskMustBeFunc ...
	ErrTaskNotRegistered = errors.New("task not registered")
	ErrTaskMustBeFunc    = errors.New("task must be a func type")
	// ErrTaskReturnsNoValue ...
	ErrTaskReturnsNoValue = errors.New("task must return at least a single value")
	// ErrLastReturnValueMustBeError ..
	ErrLastReturnValueMustBeError = errors.New("last return value of a task must be error")
)
