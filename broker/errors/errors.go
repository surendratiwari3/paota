package errors

import "errors"

var (
	ErrUnsupportedBroker   = errors.New("unsupported broker")
	ErrConnectionPoolEmpty = errors.New("connection pool is empty")
)
