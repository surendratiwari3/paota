package memory

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/broker/redis"
	"github.com/surendratiwari3/paota/schema"
	appError "github.com/surendratiwari3/paota/schema/errors"
)

func TestTaskRegistrar_RegisterTasks(t *testing.T) {
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
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)
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
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)
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
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)

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

func TestTaskRegistrar_Processor(t *testing.T) {
	mockConfigProvider := new(config.MockConfigProvider)
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)

	t.Run("AMQP Delivery", func(t *testing.T) {
		delivery := amqp.Delivery{
			Body: []byte(`{"name": "test_task"}`),
		}
		err := taskRegistrar.Processor(delivery)
		assert.Error(t, err) // Should error since task not registered
	})

	t.Run("Redis Signature", func(t *testing.T) {
		signature := &schema.Signature{
			Name: "test_task",
		}
		err := taskRegistrar.Processor(signature)
		assert.Error(t, err) // Should error since task not registered
	})

	t.Run("Unsupported Type", func(t *testing.T) {
		err := taskRegistrar.Processor("unsupported")
		assert.NoError(t, err) // Returns nil for unsupported types
	})
}

func TestTaskRegistrar_RedisMsgProcessor(t *testing.T) {
	mockConfigProvider := new(config.MockConfigProvider)
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)

	t.Run("Task Not Registered", func(t *testing.T) {
		signature := &schema.Signature{
			Name: "nonexistent_task",
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).redisMsgProcessor(signature)
		assert.Equal(t, appError.ErrTaskNotRegistered, err)
	})

	t.Run("Invalid Task Function", func(t *testing.T) {
		// Register invalid task function
		concreteRegistrar := taskRegistrar.(*DefaultTaskRegistrar)
		concreteRegistrar.registeredTasks.Store("invalid_task", "not a function")
		signature := &schema.Signature{
			Name: "invalid_task",
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).redisMsgProcessor(signature)
		assert.Equal(t, appError.ErrTaskMustBeFunc, err)
	})

	t.Run("Successful Task Execution", func(t *testing.T) {
		taskFunc := func(*schema.Signature) error { return nil }
		concreteRegistrar := taskRegistrar.(*DefaultTaskRegistrar)
		concreteRegistrar.registeredTasks.Store("success_task", taskFunc)
		signature := &schema.Signature{
			Name: "success_task",
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).redisMsgProcessor(signature)
		assert.NoError(t, err)
	})
}

func TestTaskRegistrar_RetryRedisTask(t *testing.T) {
	mockConfig := &config.Config{
		Redis: &config.RedisConfig{
			RetryCount:   3,
			RetryTimeout: 5,
		},
	}
	mockConfigProvider := new(config.MockConfigProvider)
	mockConfigProvider.On("GetConfig").Return(mockConfig, nil)

	mockRedisBroker := new(redis.RedisBroker)
	taskRegistrar := NewDefaultTaskRegistrar(mockRedisBroker, mockConfigProvider)

	t.Run("Max Retries Exceeded", func(t *testing.T) {
		signature := &schema.Signature{
			Name:        "test_task",
			RetryCount:  3,
			RetriesDone: 3,
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).retryRedisTask(signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max retry attempts reached")
	})
}

func TestTaskRegistrar_RetryTask(t *testing.T) {
	mockConfig := &config.Config{
		AMQP: &config.AMQPConfig{
			DelayedQueue: "delayed_queue",
		},
	}
	mockConfigProvider := new(config.MockConfigProvider)
	mockConfigProvider.On("GetConfig").Return(mockConfig, nil)

	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)

	t.Run("No Retry Required", func(t *testing.T) {
		signature := &schema.Signature{
			RetryCount: 0,
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).retryTask(signature)
		assert.NoError(t, err)
	})

	t.Run("Max Retries Exceeded", func(t *testing.T) {
		signature := &schema.Signature{
			RetryCount:  3,
			RetriesDone: 4,
		}
		err := taskRegistrar.(*DefaultTaskRegistrar).retryTask(signature)
		assert.NoError(t, err)
	})
}

func TestTaskRegistrar_GetRetryInterval(t *testing.T) {
	mockConfigProvider := new(config.MockConfigProvider)
	mockBroker := broker.NewMockBroker(t)
	taskRegistrar := NewDefaultTaskRegistrar(mockBroker, mockConfigProvider)

	tests := []struct {
		retryCount int
		expected   time.Duration
	}{
		{1, 1 * time.Second},
		{2, 1 * time.Second},
		{3, 2 * time.Second},
		{4, 3 * time.Second},
		{5, 5 * time.Second},
	}

	for _, tt := range tests {
		result := taskRegistrar.(*DefaultTaskRegistrar).getRetryInterval(tt.retryCount)
		assert.Equal(t, tt.expected, result)
	}
}

func TestSignatureToBytes(t *testing.T) {
	signature := &schema.Signature{
		UUID: "test-uuid",
		Name: "test-task",
		Args: []schema.Arg{
			{Type: "string", Value: "arg1"},
			{Type: "string", Value: "arg2"},
		},
	}

	bytes, err := SignatureToBytes(signature)
	assert.NoError(t, err)
	assert.NotNil(t, bytes)
	assert.Contains(t, string(bytes), "test-uuid")
	assert.Contains(t, string(bytes), "test-task")
}
