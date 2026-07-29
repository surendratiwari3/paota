package validation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

type notAFunc struct{}

func TestValidateTask(t *testing.T) {
	tests := []struct {
		name        string
		task        interface{}
		expectedErr error
	}{
		{
			name:        "valid task returning error",
			task:        func() error { return nil },
			expectedErr: nil,
		},
		{
			name:        "valid task with args returning error",
			task:        func(a string, b int) error { return nil },
			expectedErr: nil,
		},
		{
			name:        "valid task with multiple returns, last is error",
			task:        func() (string, error) { return "", nil },
			expectedErr: nil,
		},
		{
			name:        "not a function",
			task:        notAFunc{},
			expectedErr: appErrors.ErrTaskMustBeFunc,
		},
		{
			name:        "string is not a function",
			task:        "taskName",
			expectedErr: appErrors.ErrTaskMustBeFunc,
		},
		{
			name:        "function returning nothing",
			task:        func() {},
			expectedErr: appErrors.ErrTaskReturnsNoValue,
		},
		{
			name:        "last return value is not an error",
			task:        func() string { return "" },
			expectedErr: appErrors.ErrLastReturnValueMustBeError,
		},
		{
			name:        "error is not the last return value",
			task:        func() (error, string) { return nil, "" },
			expectedErr: appErrors.ErrLastReturnValueMustBeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTask(tt.task)
			if tt.expectedErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.True(t, errors.Is(err, tt.expectedErr), "expected %v, got %v", tt.expectedErr, err)
		})
	}
}

func TestValidateTaskCustomErrorType(t *testing.T) {
	type myError struct{ error }

	// A concrete type implementing error is still a valid last return value.
	assert.NoError(t, ValidateTask(func() *myError { return nil }))
}
