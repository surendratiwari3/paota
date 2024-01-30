package workergroup

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/internal/schema"
	appError "github.com/surendratiwari3/paota/internal/schema/errors"
	"github.com/surendratiwari3/paota/internal/task"
	"testing"
)

func TestProcessor(t *testing.T) {
	taskReg := task.NewMockTaskRegistrarInterface(t)
	// Create a mock task signature
	mockSignature := &schema.Signature{
		Name:       "mockTask",
		RetryCount: 3,
	}
	taskReg.On("GetRegisteredTask", mockSignature.Name).Return(nil, nil)

	wrkGrp := NewWorkerGroup(10, taskReg, "test")

	// Test case: invalid task function
	err := wrkGrp.Processor(mockSignature)
	assert.Error(t, err)
	assert.Equal(t, appError.ErrTaskNotRegistered, err)
}

func TestParseRetry(t *testing.T) {
	wrkGrp := workerGroup{}
	// Create a mock task signature
	mockSignature := &schema.Signature{
		Name:       "mockTask",
		RetryCount: 3,
	}

	// Test case: retry logic
	err := wrkGrp.parseRetry(mockSignature, errors.New("some error"))
	assert.Equal(t, mockSignature.RetriesDone, 1)
	assert.Error(t, err)
	assert.IsType(t, &appError.RetryError{}, err)

	mockSignature.RetryCount = 0
	err = wrkGrp.parseRetry(mockSignature, errors.New("some error"))
	assert.Error(t, err)
}
