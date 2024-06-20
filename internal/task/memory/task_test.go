package memory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/schema"
)

func TestTaskRegistrar_RegisterTasks(t *testing.T) {
	mockBroker := broker.NewMockBroker(t, "master")
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, nil)
	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := taskRegistrar.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)

	assert.True(t, taskRegistrar.IsTaskRegistered("taskName"))
	assert.False(t, taskRegistrar.IsTaskRegistered("taskName1"))

	// Create a mock task function
	mockTaskFuncWithReturn := func() {}
	namedTaskFuncs = map[string]interface{}{"taskName": mockTaskFuncWithReturn}

	// Register tasks
	err = taskRegistrar.RegisterTasks(namedTaskFuncs)
	assert.NotNil(t, err)
}

func TestTaskRegistrar_IsTaskRegistered(t *testing.T) {
	mockBroker := broker.NewMockBroker(t, "master")
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, nil)
	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := taskRegistrar.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)
	assert.True(t, taskRegistrar.IsTaskRegistered("taskName"))
	assert.False(t, taskRegistrar.IsTaskRegistered("taskName1"))
}

func TestTaskRegistrar_GetRegisteredTask(t *testing.T) {
	mockBroker := broker.NewMockBroker(t, "master")
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, nil)

	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := taskRegistrar.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)

	task, err := taskRegistrar.GetRegisteredTask("taskName")
	assert.Nil(t, err)
	assert.NotNil(t, task)

	task, err = taskRegistrar.GetRegisteredTask("taskName1")
	assert.NotNil(t, err)
	assert.Nil(t, task)
}

func TestTaskRegistrar_SendTaskWithContext(t *testing.T) {
	mockBroker := broker.NewMockBroker(t, "master")
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, nil)
	mockBroker.On("Publish", mock.Anything, mock.Anything).Return(nil)
	// Create a mock task signature
	mockSignature := &schema.Signature{
		UUID: "mockUUID",
	}
	err := taskRegistrar.SendTask(mockSignature)
	assert.Nil(t, err)

	mockBroker = broker.NewMockBroker(t, "master")
	taskRegistrar = NewDefaultTaskRegistrar(mockBroker, nil)
	mockBroker.On("Publish", mock.Anything, mock.Anything).Return(errors.New("test error"))
	err = taskRegistrar.SendTask(mockSignature)
	assert.NotNil(t, err)
}
