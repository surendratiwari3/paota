package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/workergroup"
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
	broker, err := NewRedisBroker(mockConfigProvider)

	// Assertions
	assert.Nil(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, "test_queue", broker.(*RedisBroker).queue, "Queue name should match the config")
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
		queue:    queue,
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
		queue:    queue,
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
