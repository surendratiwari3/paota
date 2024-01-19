package errors

import "errors"

var (
	ErrUnsupportedStore = errors.New("unsupported store")
	// Add more specific errors related to the broker package
)
