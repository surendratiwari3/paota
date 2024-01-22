package errors

import "errors"

var (
	ErrUnsupportedBroker     = errors.New("unsupported broker")
	ErrConnectionPoolEmpty   = errors.New("connection pool is empty")
	ErrInvalidConfig         = errors.New("config is invalid")
	ErrNilConfig             = errors.New("config is nil")
	ErrInvalidConnectionPool = errors.New("connection pool is invalid")
)
