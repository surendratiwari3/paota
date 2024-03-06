package errors

import "errors"

var (
	ErrInvalidConfig = errors.New("config is invalid")
	ErrNilConfig     = errors.New("config is nil")
)
