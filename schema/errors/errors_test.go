package errors

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryErrorMessage(t *testing.T) {
	cause := errors.New("connection refused")
	retryErr := &RetryError{
		RetryCount: 3,
		RetryAfter: 5 * time.Second,
		Err:        cause,
	}

	msg := retryErr.Error()

	assert.Contains(t, msg, "3")
	assert.Contains(t, msg, "5s")
	assert.Contains(t, msg, "connection refused")
}

func TestRetryErrorIsAnError(t *testing.T) {
	var err error = &RetryError{RetryCount: 1, Err: errors.New("boom")}
	assert.Error(t, err)
}

// TestSentinelErrorsAreDistinct guards against a copy-paste that would make two
// sentinels compare equal and silently collapse two failure modes into one.
func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrUnsupportedBroker":          ErrUnsupportedBroker,
		"ErrEmptyMessage":               ErrEmptyMessage,
		"ErrDeliveriesChannelClosed":    ErrDeliveriesChannelClosed,
		"ErrInvalidConfig":              ErrInvalidConfig,
		"ErrNilConfig":                  ErrNilConfig,
		"ErrConnectionPoolEmpty":        ErrConnectionPoolEmpty,
		"ErrInvalidConnectionPool":      ErrInvalidConnectionPool,
		"ErrConnectionPoolClosed":       ErrConnectionPoolClosed,
		"ErrUnsupportedStore":           ErrUnsupportedStore,
		"ErrTaskNotRegistered":          ErrTaskNotRegistered,
		"ErrTaskMustBeFunc":             ErrTaskMustBeFunc,
		"ErrTaskReturnsNoValue":         ErrTaskReturnsNoValue,
		"ErrLastReturnValueMustBeError": ErrLastReturnValueMustBeError,
		"ErrUnregisteredTask":           ErrUnregisteredTask,
		"ErrTaskFunctionNotFound":       ErrTaskFunctionNotFound,
	}

	seen := make(map[string]string, len(sentinels))
	for name, err := range sentinels {
		assert.Error(t, err)
		if other, dup := seen[err.Error()]; dup {
			t.Errorf("%s and %s share the message %q", name, other, err.Error())
		}
		seen[err.Error()] = name
	}
}

func TestSentinelErrorsWrapCorrectly(t *testing.T) {
	wrapped := fmt.Errorf("publishing task: %w", ErrEmptyMessage)

	assert.True(t, errors.Is(wrapped, ErrEmptyMessage))
	assert.False(t, errors.Is(wrapped, ErrNilConfig))
}
