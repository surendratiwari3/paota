package errors

import "errors"

var (
	ErrConnectionPoolEmpty   = errors.New("connection pool is empty")
	ErrInvalidConnectionPool = errors.New("connection pool is invalid")
)
