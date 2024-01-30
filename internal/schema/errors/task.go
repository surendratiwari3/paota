package errors

import "errors"

var (
	// ErrTaskMustBeFunc ...
	ErrTaskNotRegistered = errors.New("Task not registered")
	ErrTaskMustBeFunc    = errors.New("Task must be a func type")
	// ErrTaskReturnsNoValue ...
	ErrTaskReturnsNoValue = errors.New("Task must return at least a single value")
	// ErrLastReturnValueMustBeError ..
	ErrLastReturnValueMustBeError = errors.New("Last return value of a task must be error")
)
