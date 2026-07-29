package errors

import "errors"

var (
	ErrConnectionPoolEmpty   = errors.New("connection pool is empty")
	ErrInvalidConnectionPool = errors.New("connection pool is invalid")
	// ErrConnectionPoolClosed is returned once the provider has been closed and
	// no further connections will be handed out.
	ErrConnectionPoolClosed = errors.New("connection pool is closed")
)
