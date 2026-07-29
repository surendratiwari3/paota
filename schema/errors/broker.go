package errors

import "errors"

var (
	ErrUnsupportedBroker = errors.New("unsupported broker")
	ErrEmptyMessage      = errors.New("received an empty message")
	// ErrDeliveriesChannelClosed is returned when RabbitMQ closes the delivery
	// channel underneath a running consumer, which means the consumer has to be
	// restarted.
	ErrDeliveriesChannelClosed = errors.New("deliveries channel closed")
)
