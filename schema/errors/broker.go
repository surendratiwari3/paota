package errors

import "errors"

var (
	ErrUnsupportedBroker = errors.New("unsupported broker")
	ErrEmptyMessage      = errors.New("received an empty message")
)
