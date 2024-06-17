package errors

import "errors"

var (
	ErrUnsupportedBroker     = errors.New("unsupported broker")
	ErrEmptyMessage          = errors.New("received an empty message")
	ErrUnsupportedBrokerType = errors.New("unsupported broker type")
)
