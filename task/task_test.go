package _default

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTaskRegistrar_RegisterTasks(t *testing.T) {

	taskRegistrar := NewDefaultTaskRegistrar()
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

func TestWorkerPool_IsTaskRegistered(t *testing.T) {
	taskRegistrar := NewDefaultTaskRegistrar()
	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := taskRegistrar.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)
	assert.True(t, taskRegistrar.IsTaskRegistered("taskName"))
	assert.False(t, taskRegistrar.IsTaskRegistered("taskName1"))
}

func TestWorkerPool_GetRegisteredTask(t *testing.T) {
	taskRegistrar := NewDefaultTaskRegistrar()

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
