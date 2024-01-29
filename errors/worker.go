package errors

import (
	"errors"
)

var (
	ErrUnregisteredTask     = errors.New("unregistered task")
	ErrTaskFunctionNotFound = errors.New("task function not found")
)
