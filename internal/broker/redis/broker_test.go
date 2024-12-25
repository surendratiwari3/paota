package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"

	"github.com/gomodule/redigo/redis"
	"github.com/surendratiwari3/paota/schema"
)

// MockRedisProviderInterface is a mock implementation of the RedisProviderInterface.
type MockRedisProviderInterface struct {
	mock.Mock
}

func (m *MockRedisProviderInterface) Publish(ctx context.Context, queue string, signature *schema.Signature) error {
	args := m.Called(ctx, queue, signature)
	return args.Error(0)
}

func (m *MockRedisProviderInterface) Subscribe(queue string, handler func(*schema.Signature) error) error {
	args := m.Called(queue, handler)
	return args.Error(0)
}

func (m *MockRedisProviderInterface) CloseConnection() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisProviderInterface) GetConn() redis.Conn {
	args := m.Called()
	return args.Get(0).(redis.Conn)
}

// MockRedisConn mocks redis.Conn
type MockRedisConn struct {
	mock.Mock
}

func (m *MockRedisConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisConn) Err() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisConn) Do(commandName string, args ...interface{}) (interface{}, error) {
	mockArgs := append([]interface{}{commandName}, args...)
	result := m.Called(mockArgs...)
	return result.Get(0), result.Error(1)
}

func (m *MockRedisConn) Send(commandName string, args ...interface{}) error {
	mockArgs := append([]interface{}{commandName}, args...)
	return m.Called(mockArgs...).Error(0)
}

func (m *MockRedisConn) Flush() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisConn) Receive() (interface{}, error) {
	args := m.Called()
	return args.Get(0), args.Error(1)
}

func TestNewRedisBroker(t *testing.T) {
	mockConfigProvider := new(config.MockConfigProvider)

	redisConfig := &config.RedisConfig{
		Address: "localhost:6379",
		DelayedTasksKey: "delayed_tasks",
	}

	conf := &config.Config{
		Broker:        "redis",
		TaskQueueName: "test_queue",
		Redis:         redisConfig,
	}

	mockConfigProvider.On("GetConfig").Return(conf, nil)

	// Create a new instance of RedisBroker
	broker, err := NewRedisBroker(nil, conf)

	// Assertions
	assert.Nil(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, "test_queue", broker.(*RedisBroker).config.TaskQueueName)
	assert.Equal(t, "redis", broker.BrokerType())
	assert.True(t, broker.(*RedisBroker).running)
}

func TestRedisBrokerPublish(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockConn := new(MockRedisConn)
	queue := "test_queue"
	ctx := context.Background()

	// Test immediate task
	t.Run("Immediate Task", func(t *testing.T) {
		signature := &schema.Signature{
			Name: "TestTask",
			Args: []schema.Arg{
				{Type: "string", Value: "test_value"},
			},
		}

		mockProvider.On("GetConn").Return(mockConn)
		mockConn.On("Do", "LPUSH", queue, mock.Anything).Return(int64(1), nil)
		mockConn.On("Close").Return(nil)

		redisBroker := &RedisBroker{
			provider: mockProvider,
			config: &config.Config{
				TaskQueueName: queue,
			},
		}

		err := redisBroker.Publish(ctx, signature)
		require.NoError(t, err)
	})

	// Test delayed task
	t.Run("Delayed Task", func(t *testing.T) {
		futureTime := time.Now().Add(1 * time.Hour)
		signature := &schema.Signature{
			Name: "DelayedTask",
			ETA:  &futureTime,
		}

		mockProvider.On("GetConn").Return(mockConn)
		mockConn.On("Do", "ZADD", mock.Anything, mock.Anything, mock.Anything).Return(int64(1), nil)
		mockConn.On("Close").Return(nil)

		redisBroker := &RedisBroker{
			provider: mockProvider,
			config: &config.Config{
				TaskQueueName: queue,
				Redis: &config.RedisConfig{
					DelayedTasksKey: "delayed_tasks",
				},
			},
		}

		err := redisBroker.Publish(ctx, signature)
		require.NoError(t, err)
	})
}

func TestGetTaskDelay(t *testing.T) {
	rb := &RedisBroker{}

	t.Run("Future Task", func(t *testing.T) {
		futureTime := time.Now().Add(1 * time.Hour)
		sig := &schema.Signature{ETA: &futureTime}
		delay := rb.getTaskDelay(sig)
		assert.True(t, delay > 0)
		assert.True(t, delay <= time.Hour)
	})

	t.Run("Past Task", func(t *testing.T) {
		pastTime := time.Now().Add(-1 * time.Hour)
		sig := &schema.Signature{ETA: &pastTime}
		delay := rb.getTaskDelay(sig)
		assert.Equal(t, time.Duration(0), delay)
	})

	t.Run("No ETA", func(t *testing.T) {
		sig := &schema.Signature{}
		delay := rb.getTaskDelay(sig)
		assert.Equal(t, time.Duration(0), delay)
	})
}

func TestPublishToMainQueue(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockConn := new(MockRedisConn)
	queue := "test_queue"

	signature := &schema.Signature{
		Name: "TestTask",
		Args: []schema.Arg{
			{Type: "string", Value: "test_value"},
		},
	}

	mockProvider.On("GetConn").Return(mockConn)
	mockConn.On("Do", "LPUSH", queue, mock.Anything).Return(int64(1), nil)
	mockConn.On("Close").Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
		config: &config.Config{
			TaskQueueName: queue,
		},
	}

	err := redisBroker.publishToMainQueue(context.Background(), signature)
	require.NoError(t, err)
	mockProvider.AssertExpectations(t)
	mockConn.AssertExpectations(t)
}

func TestStartDelayedTasksProcessor(t *testing.T) {
	redisBroker := &RedisBroker{
		stopChan: make(chan struct{}),
	}

	redisBroker.StartDelayedTasksProcessor()
	assert.NotNil(t, redisBroker.stopChan)

	// Cleanup
	close(redisBroker.stopChan)
}

func TestStop(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockProvider.On("CloseConnection").Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
		stopChan: make(chan struct{}),
	}

	err := redisBroker.Stop()
	require.NoError(t, err)
	mockProvider.AssertExpectations(t)

	// Verify channel is closed
	select {
	case <-redisBroker.stopChan:
		// Channel is closed as expected
	default:
		t.Error("stopChan should be closed")
	}
}

func TestGetProvider(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	redisBroker := &RedisBroker{
		provider: mockProvider,
	}

	provider := redisBroker.GetProvider()
	assert.Equal(t, mockProvider, provider)
}

func TestRedisBrokerProcessDelayedTasks(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockConn := new(MockRedisConn)

	redisBroker := &RedisBroker{
		provider: mockProvider,
		config: &config.Config{
			TaskQueueName: "main_queue",
			Redis: &config.RedisConfig{
				DelayedTasksKey: "delayed_queue",
			},
		},
		stopChan: make(chan struct{}),
		running:  true,
	}

	mockProvider.On("GetConn").Return(mockConn)
	mockConn.On("Do", "ZRANGEBYSCORE", "delayed_queue", "-inf", mock.Anything).Return([]interface{}{[]byte(`{"name":"test_task"}`)}, nil)
	mockConn.On("Send", "MULTI").Return(nil)
	mockConn.On("Send", "LPUSH", "main_queue", mock.Anything).Return(nil)
	mockConn.On("Send", "ZREM", "delayed_queue", mock.Anything).Return(nil)
	mockConn.On("Do", "EXEC").Return([]interface{}{int64(1), int64(1)}, nil)
	mockConn.On("Close").Return(nil)

	go redisBroker.processDelayedTasks()
	time.Sleep(2 * time.Second)
	close(redisBroker.stopChan)

	mockProvider.AssertExpectations(t)
	mockConn.AssertExpectations(t)
}
