package workerpool

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/factory"
	"github.com/surendratiwari3/paota/internal/schema"
	"github.com/surendratiwari3/paota/internal/task"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	// Mock the broker for testing
	mockBroker := broker.NewMockBroker(t)
	mockTaskRegistrar := task.NewMockTaskRegistrarInterface(t)
	// Create a mock AMQPAdapter with a valid config
	mockConfigProvider := new(config.MockConfigProvider)
	mockConfigProvider.On("GetConfig").Return(&config.Config{
		Broker:        "amqp",
		TaskQueueName: "test",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 2,
		},
	}, nil)

	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar").Return(mockTaskRegistrar)
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

func TestWorkerPool_SendTaskWithContext(t *testing.T) {
	mockBroker := &broker.MockBroker{}
	mockBroker.On("Publish", mock.Anything, mock.Anything).Return(nil)
	// Create a new WorkerPool
	wp := &WorkerPool{
		broker: mockBroker,
	}

	// Create a mock task signature
	mockSignature := &schema.Signature{
		UUID: "mockUUID",
	}

	// Send the task with context
	state, err := wp.SendTaskWithContext(context.Background(), mockSignature)

	// Assert that the task is sent successfully
	assert.Nil(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, "Pending", state.Status)

	mockBrokerFailed := &broker.MockBroker{}
	mockBrokerFailed.On("Publish", mock.Anything, mock.Anything).Return(errors.New("failed"))
	wp.broker = mockBrokerFailed
	mockSignature.UUID = ""
	// Send the task with context
	state, err = wp.SendTaskWithContext(context.Background(), mockSignature)

	// Assert that the task is sent successfully
	assert.NotNil(t, err)
	assert.Nil(t, state)
}

func TestWorkerPool_StartWithBrokerInError(t *testing.T) {
	mockBroker := broker.NewMockBroker(t)
	mockTaskReg := task.NewMockTaskRegistrarInterface(t)

	mockBroker.On("StartConsumer", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("start consumer failed"))
	mockBroker.On("StopConsumer").Return()
	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar").Return(mockTaskReg)
	mockTaskReg.On("GetRegisteredTaskCount").Return(uint(10))
	wp := &WorkerPool{
		broker:        mockBroker,
		started:       true,
		concurrency:   10,
		nameSpace:     "test",
		taskRegistrar: mockTaskReg,
		factory:       mockFactory,
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
}

func TestWorkerPool_StartWithBroker(t *testing.T) {
	mockBrokerWithOutError := broker.NewMockBroker(t)
	mockTaskReg := task.NewMockTaskRegistrarInterface(t)

	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBrokerWithOutError, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar").Return(mockTaskReg)
	mockTaskReg.On("GetRegisteredTaskCount").Return(uint(10))

	mockBrokerWithOutError.On("StartConsumer", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("start consumer failed"))
	mockBrokerWithOutError.On("StopConsumer").Return()

	wp := &WorkerPool{
		broker:        mockBrokerWithOutError,
		started:       false,
		concurrency:   10,
		nameSpace:     "test",
		taskRegistrar: mockTaskReg,
		factory:       mockFactory,
	}

	wp.broker = mockBrokerWithOutError
	go func() {
		wp.started = false
		err := wp.Start()
		assert.Nil(t, err)
	}()

	time.Sleep(100 * time.Millisecond)
	wp.Stop()
}
