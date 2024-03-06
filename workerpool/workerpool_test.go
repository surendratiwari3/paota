package workerpool

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/factory"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/schema"
	"os"
	"testing"
	"time"
)

func setupConfigProvider() {
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
	config.SetConfigProvider(mockConfigProvider)
}

func TestNewWorkerPool(t *testing.T) {
	setupConfigProvider()
	// Mock the broker for testing
	mockBroker := broker.NewMockBroker(t)
	mockTaskRegistrar := task.NewMockTaskRegistrarInterface(t)

	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar", mock.Anything).Return(mockTaskRegistrar)

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
	setupConfigProvider()
	mockBroker := &broker.MockBroker{}

	mockBroker.On("Publish", mock.Anything, mock.Anything).Return(nil)

	// Mock the broker for testing
	mockTaskRegistrar := task.NewMockTaskRegistrarInterface(t)
	mockTaskRegistrar.On("SendTaskWithContext", mock.Anything, mock.Anything).Return(nil)

	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar", mock.Anything).Return(mockTaskRegistrar)

	globalFactory = mockFactory

	pool, err := NewWorkerPool(context.Background(), 10, "test")
	if err != nil {
		t.Error("Broker not created correctly", err)
	}
	if pool == nil {
		t.Error("Broker not created correctly", err)
	}

	// Create a mock task signature
	mockSignature := &schema.Signature{
		UUID: "mockUUID",
	}

	// Send the task with context
	state, err := pool.SendTaskWithContext(context.Background(), mockSignature)

	// Assert that the task is sent successfully
	assert.Nil(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, "Pending", state.Status)
}

func TestWorkerPool_Start(t *testing.T) {
	setupConfigProvider()
	mockBroker := broker.NewMockBroker(t)
	mockBroker.On("StartConsumer", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("start consumer failed"))
	mockBroker.On("StopConsumer").Return()

	mockTaskReg := task.NewMockTaskRegistrarInterface(t)
	mockTaskReg.On("GetRegisteredTaskCount").Return(uint(10))

	mockFactory := new(factory.MockIFactory)
	mockFactory.On("CreateBroker").Return(mockBroker, nil)
	mockFactory.On("CreateStore").Return(nil)
	mockFactory.On("CreateTaskRegistrar", mock.Anything).Return(mockTaskReg)

	globalFactory = mockFactory

	wp, err := NewWorkerPool(context.Background(), 10, "test")
	if err != nil {
		t.Error("Broker not created correctly", err)
	}
	if wp == nil {
		t.Error("Broker not created correctly", err)
	}

	go func() {
		err = wp.Start()
	}()
	time.Sleep(100 * time.Millisecond)
	wp.Stop()
	assert.Nil(t, err)
}

func TestMain(m *testing.M) {
	// Perform setup tasks here

	// Run tests
	exitCode := m.Run()

	// Perform cleanup tasks here

	// Exit with the same exit code as the tests
	os.Exit(exitCode)
}
