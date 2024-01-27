package workerpool

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/mocks"
	"github.com/surendratiwari3/paota/task"
	"sync"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	// Mock the broker for testing
	mockBroker := &mocks.Broker{}
	// Create a mock AMQPAdapter with a valid config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfigProvider.On("GetConfig").Return(&config.Config{
		Broker:        "amqp",
		TaskQueueName: "test",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 2,
		},
	}, nil)

	mockFactory := new(mocks.IFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	config.SetConfigProvider(mockConfigProvider)

	globalFactory = mockFactory

	pool, err := NewWorkerPool(context.Background(), 10, "test")
	if err != nil {
		t.Error("Broker not created correctly", err)
	}
	if pool == nil {
		t.Error("Broker not created correctly", err)
	}
}

func TestWorkerPool_RegisterTasks(t *testing.T) {
	// Create a new WorkerPool
	wp := &WorkerPool{
		registeredTasks: new(sync.Map),
	}

	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := wp.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)

	assert.True(t, wp.IsTaskRegistered("taskName"))
	assert.False(t, wp.IsTaskRegistered("taskName1"))

	// Create a mock task function
	mockTaskFuncWithReturn := func() {}
	namedTaskFuncs = map[string]interface{}{"taskName": mockTaskFuncWithReturn}

	// Register tasks
	err = wp.RegisterTasks(namedTaskFuncs)
	assert.NotNil(t, err)
}

func TestWorkerPool_IsTaskRegistered(t *testing.T) {
	// Create a new WorkerPool
	wp := &WorkerPool{
		registeredTasks: new(sync.Map),
	}

	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := wp.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)
	assert.True(t, wp.IsTaskRegistered("taskName"))
	assert.False(t, wp.IsTaskRegistered("taskName1"))
}

func TestWorkerPool_GetRegisteredTask(t *testing.T) {
	// Create a new WorkerPool
	wp := &WorkerPool{
		registeredTasks: new(sync.Map),
	}

	// Create a mock task function
	mockTaskFunc := func() error { return nil }
	namedTaskFuncs := map[string]interface{}{"taskName": mockTaskFunc}

	// Register tasks
	err := wp.RegisterTasks(namedTaskFuncs)
	assert.Nil(t, err)

	task, err := wp.GetRegisteredTask("taskName")
	assert.Nil(t, err)
	assert.NotNil(t, task)

	task, err = wp.GetRegisteredTask("taskName1")
	assert.NotNil(t, err)
	assert.Nil(t, task)
}

func TestWorkerPool_SendTaskWithContext(t *testing.T) {
	mockBroker := &mocks.Broker{}
	mockBroker.On("Publish", mock.Anything, mock.Anything).Return(nil)
	// Create a new WorkerPool
	wp := &WorkerPool{
		broker: mockBroker,
	}

	// Create a mock task signature
	mockSignature := &task.Signature{
		UUID: "mockUUID",
	}

	// Send the task with context
	state, err := wp.SendTaskWithContext(context.Background(), mockSignature)

	// Assert that the task is sent successfully
	assert.Nil(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, "Pending", state.Status)

	mockBrokerFailed := &mocks.Broker{}
	mockBrokerFailed.On("Publish", mock.Anything, mock.Anything).Return(errors.New("failed"))
	wp.broker = mockBrokerFailed
	mockSignature.UUID = ""
	// Send the task with context
	state, err = wp.SendTaskWithContext(context.Background(), mockSignature)

	// Assert that the task is sent successfully
	assert.NotNil(t, err)
	assert.Nil(t, state)
}

func TestWorkerPool_Start(t *testing.T) {
	mockBroker := mocks.NewBroker(t)
	mockBroker.On("StartConsumer", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("start consumer failed"))
	wp := &WorkerPool{
		broker:          mockBroker,
		started:         true,
		concurrency:     10,
		nameSpace:       "test",
		registeredTasks: new(sync.Map),
	}

	err := wp.Start()
	assert.Nil(t, err)

	go func() {
		wp.started = false
		err = wp.Start()
		assert.Nil(t, err)
	}()
	time.Sleep(100 * time.Millisecond)
	wp.Stop()
	wp.started = false
	wp.Stop()

	mockBrokerWithOutError := mocks.NewBroker(t)
	mockBrokerWithOutError.On("StartConsumer", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("start consumer failed"))
	wp.broker = mockBrokerWithOutError
	go func() {
		wp.started = false
		err = wp.Start()
		assert.Nil(t, err)
	}()

	time.Sleep(100 * time.Millisecond)
	wp.Stop()
}
