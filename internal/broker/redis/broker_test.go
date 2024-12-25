package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/workergroup"

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
	}

	conf := &config.Config{
		Broker:        "redis",
		TaskQueueName: "test_queue",
		Redis:         redisConfig,
	}

	mockConfigProvider.On("GetConfig").Return(conf, nil)

	// Create a new instance of RedisBroker
	broker, err := NewRedisBroker(nil, mockConfigProvider.GetConfig())

	// Assertions
	assert.Nil(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, "test_queue", broker.(*RedisBroker).config.TaskQueueName, "Queue name should match the config")
	assert.Equal(t, "redis", broker.BrokerType(), "BrokerType should be redis")
}

func TestRedisBrokerPublish(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	queue := "test_queue"
	ctx := context.Background()

	signature := &schema.Signature{
		Name: "TestTask",
		Args: []schema.Arg{
			{
				Type:  "string",
				Value: "test_value",
			},
		},
	}

	mockProvider.On("Publish", ctx, queue, signature).Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
		config: &config.Config{
			TaskQueueName: queue,
		},
	}

	err := redisBroker.Publish(ctx, signature)

	// Assertions
	require.NoError(t, err)
	mockProvider.AssertCalled(t, "Publish", ctx, queue, signature)
}

func TestRedisBrokerStartConsumer(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockWorkerGroup := new(workergroup.MockWorkerGroupInterface)
	queue := "test_queue"

	signature := &schema.Signature{
		Name: "TestTask",
	}

	mockProvider.On("Subscribe", queue, mock.Anything).Run(func(args mock.Arguments) {
		// Simulate calling the handler
		handler := args.Get(1).(func(*schema.Signature) error)
		handler(signature)
	}).Return(nil)

	mockWorkerGroup.On("AssignJob", signature).Return()

	redisBroker := &RedisBroker{
		provider: mockProvider,
		config: &config.Config{
			TaskQueueName: queue,
		},
	}

	err := redisBroker.StartConsumer(context.Background(), mockWorkerGroup)

	// Assertions
	require.NoError(t, err)
	mockProvider.AssertCalled(t, "Subscribe", queue, mock.Anything)
	mockWorkerGroup.AssertCalled(t, "AssignJob", signature)
}

func TestRedisBrokerStopConsumer(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)

	mockProvider.On("CloseConnection").Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
	}

	redisBroker.StopConsumer()

	// Assertions
	mockProvider.AssertCalled(t, "CloseConnection")
}

func TestRedisBrokerLPush(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockConn := new(MockRedisConn)

	mockProvider.On("GetConn").Return(mockConn)
	mockConn.On("Do", "LPUSH", "test_key", "test_value").Return(int64(1), nil)
	mockConn.On("Close").Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
	}

	err := redisBroker.LPush(context.Background(), "test_key", "test_value")

	require.NoError(t, err)
	mockProvider.AssertCalled(t, "GetConn")
	mockConn.AssertCalled(t, "Do", "LPUSH", "test_key", "test_value")
	mockConn.AssertCalled(t, "Close")
}

func TestRedisBrokerBLPop(t *testing.T) {
	mockProvider := new(MockRedisProviderInterface)
	mockConn := new(MockRedisConn)

	mockProvider.On("GetConn").Return(mockConn)
	mockConn.On("Do", "BLPOP", "test_key", 0).Return([]interface{}{[]byte("test_key"), []byte("test_value")}, nil)
	mockConn.On("Close").Return(nil)

	redisBroker := &RedisBroker{
		provider: mockProvider,
	}

	result, err := redisBroker.BLPop(context.Background(), "test_key")

	require.NoError(t, err)
	require.NotNil(t, result)
	mockProvider.AssertCalled(t, "GetConn")
	mockConn.AssertCalled(t, "Do", "BLPOP", "test_key", 0)
	mockConn.AssertCalled(t, "Close")
}

func TestRedisBrokerClose(t *testing.T) {
	redisBroker := &RedisBroker{
		running:  true,
		stopChan: make(chan struct{}),
	}

	err := redisBroker.Close()

	require.NoError(t, err)
	assert.False(t, redisBroker.running)

	// Verify channel is closed by trying to send to it
	select {
	case <-redisBroker.stopChan:
		// Channel is closed as expected
	default:
		t.Error("stopChan should be closed")
	}
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

	// Start the delayed tasks processor
	go redisBroker.processDelayedTasks()

	// Let it run for a short time
	time.Sleep(2 * time.Second)

	// Stop the processor
	close(redisBroker.stopChan)

	mockProvider.AssertCalled(t, "GetConn")
	mockConn.AssertCalled(t, "Do", "ZRANGEBYSCORE", "delayed_queue", "-inf", mock.Anything)
	mockConn.AssertCalled(t, "Close")
}
