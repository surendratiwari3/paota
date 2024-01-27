package workerpool

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/mocks"
	"sync"
	"testing"
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

	if taskFunc, ok := wp.registeredTasks.Load("taskName"); ok {
		assert.NotNil(t, taskFunc)
	} else {
		t.Error("function is not present in sync map")
	}

	assert.True(t, wp.IsTaskRegistered("taskName"))
	assert.False(t, wp.IsTaskRegistered("taskName1"))

	task, err := wp.GetRegisteredTask("taskName")
	assert.Nil(t, err)
	assert.NotNil(t, task)

	task, err = wp.GetRegisteredTask("taskName1")
	assert.NotNil(t, err)
	assert.Nil(t, task)

	// Create a mock task function
	mockTaskFuncWithReturn := func() {}
	namedTaskFuncs = map[string]interface{}{"taskName": mockTaskFuncWithReturn}

	// Register tasks
	err = wp.RegisterTasks(namedTaskFuncs)
	assert.NotNil(t, err)
}
