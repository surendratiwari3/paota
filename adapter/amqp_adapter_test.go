package adapter

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/mocks"
	"testing"
)

func TestNewAMQPAdapterWithNilConfig(t *testing.T) {
	// Create a mock ConfigProvider with a nil config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfigProvider.On("GetConfig").Return(nil)

	// Set the mock ConfigProvider for testing
	config.SetConfigProvider(mockConfigProvider)

	// Create a new AMQPAdapter
	adapter := NewAMQPAdapter()

	// Assert that the adapter is nil since the config is nil
	assert.Nil(t, adapter)

	// Verify that the GetConfig method was called
	mockConfigProvider.AssertExpectations(t)
}

func TestNewAMQPAdapterWithValidConfig(t *testing.T) {
	// Create a mock ConfigProvider with a valid config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfig := &config.Config{AMQP: &config.AMQPConfig{}}
	mockConfigProvider.On("GetConfig").Return(mockConfig)

	// Set the mock ConfigProvider for testing
	config.SetConfigProvider(mockConfigProvider)

	// Create a new AMQPAdapter
	adapter := NewAMQPAdapter()

	// Assert that the adapter is not nil
	assert.NotNil(t, adapter)

	// Verify that the GetConfig method was called
	mockConfigProvider.AssertExpectations(t)
}

// TestAMQPAdapter tests the functionality of the AMQPAdapter
func TestCreateConnection_ConnectionRefused(t *testing.T) {
	// Replace this with your actual AMQP configuration
	amqpConfig := &config.Config{
		Broker: "amqp",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://guest:guest@localhost:5672/",
			HeartBeatInterval:  10,
			ConnectionPoolSize: 3,
		},
	}

	amqpAdapter := AMQPAdapter{amqpConfig: amqpConfig.AMQP}

	t.Run("TestCreateConnection", func(t *testing.T) {
		conn, err := amqpAdapter.CreateConnection()
		assert.Error(t, err, "dial tcp [::1]:5672: connect: connection refused")
		assert.Nil(t, conn)
		defer func() {
			if conn != nil {
				conn.Close()
			}
		}()
	})
}

func TestCreateConnection_NilAMQPConfig(t *testing.T) {
	// Create a mock AMQPAdapter with a valid config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfigProvider.On("GetConfig").Return(&config.Config{}, nil)

	config.SetConfigProvider(mockConfigProvider)

	adapter := AMQPAdapter{}

	// Call the CreateConnection method
	conn, err := adapter.CreateConnection()

	// Assert that the connection is not nil and there is no error
	assert.Nil(t, conn)
	assert.Error(t, err, errors.ErrNilConfig.Error())
}

func TestCreateConnectionPool_ConnectionError(t *testing.T) {
	// Create a mock AMQPAdapter with a valid config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfigProvider.On("GetConfig").Return(&config.Config{
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 2,
		},
	}, nil)
	config.SetConfigProvider(mockConfigProvider)
	adapter := NewAMQPAdapter()
	err := adapter.CreateConnectionPool()
	// Assert that there is no error
	assert.Error(t, err, "dial tcp [::1]:5672: connect: connection refused")
}

func TestCreateConnectionPool_InvalidConnectionPoolSize(t *testing.T) {
	// Create a mock AMQPAdapter with a valid config
	mockConfigProvider := new(mocks.ConfigProvider)
	mockConfigProvider.On("GetConfig").Return(&config.Config{
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 1,
		},
	}, nil)
	config.SetConfigProvider(mockConfigProvider)
	adapter := NewAMQPAdapter()
	err := adapter.CreateConnectionPool()
	// Assert that there is no error
	assert.Error(t, err, errors.ErrInvalidConfig.Error())
}

func TestReleaseConnectionToPool(t *testing.T) {
	// Set up the AMQPAdapter
	adapter := &AMQPAdapter{
		ConnectionPool: []*amqp.Connection{},
	}

	// Create a mock connection
	mockConnection := new(amqp.Connection)

	// Release the mock connection to the pool
	err := adapter.ReleaseConnectionToPool(mockConnection)

	// Assert that there is no error
	assert.NoError(t, err)
	// Assert that the ConnectionPool is not nil and has one connection
	assert.NotNil(t, adapter.ConnectionPool)
	assert.Equal(t, 1, len(adapter.ConnectionPool))
}

func TestGetConnectionFromPool(t *testing.T) {
	// Case 1: Non-empty connection pool
	adapter := &AMQPAdapter{
		ConnectionPool: []*amqp.Connection{new(amqp.Connection)},
	}

	// Call the GetConnectionFromPool method
	conn, err := adapter.GetConnectionFromPool()

	// Assert that there is no error
	assert.NoError(t, err)
	// Assert that the returned connection is not nil
	assert.NotNil(t, conn)
	// Assert that the ConnectionPool is empty
	assert.Equal(t, 0, len(adapter.ConnectionPool))

	// Case 2: Empty connection pool
	emptyAdapter := &AMQPAdapter{
		ConnectionPool: nil,
	}

	// Call the GetConnectionFromPool method on an empty pool
	emptyConn, emptyErr := emptyAdapter.GetConnectionFromPool()

	// Assert that there is an error
	assert.Error(t, emptyErr)
	// Assert that the returned connection is nil
	assert.Nil(t, emptyConn)
	// Assert that the correct error is returned
	assert.Equal(t, errors.ErrConnectionPoolEmpty, emptyErr)
}
