package amqp

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"sync"
	"testing"
)

func TestAMQPBrokerGetRoutingKey(t *testing.T) {
	cfg := &config.Config{
		AMQP: &config.AMQPConfig{
			ExchangeType: "direct",
			BindingKey:   "test_key",
		},
		TaskQueueName: "test_queue",
	}

	broker := &AMQPBroker{
		Config: cfg,
	}

	routingKey := broker.getRoutingKey()
	require.Equal(t, "test_key", routingKey, "Routing key should match the direct exchange binding key")

	cfg.AMQP.ExchangeType = "fanout"
	routingKey = broker.getRoutingKey()
	require.Equal(t, "test_queue", routingKey, "Routing key should match the task queue name")
}

func TestIsDirectExchange(t *testing.T) {
	// Prepare a sample AMQPBroker instance with a direct exchange
	cfg := &config.Config{
		AMQP: &config.AMQPConfig{
			ExchangeType: "direct",
		},
	}
	amqpBroker := &AMQPBroker{
		Config: cfg,
	}

	// Check if the exchange type is direct
	isDirect := amqpBroker.isDirectExchange()
	if !isDirect {
		t.Error("Expected exchange type to be direct, got false")
	}

	// Prepare another sample AMQPBroker instance with a different exchange type
	cfgNonDirect := &config.Config{
		AMQP: &config.AMQPConfig{
			ExchangeType: "fanout",
		},
	}
	amqpBrokerNonDirect := &AMQPBroker{
		Config: cfgNonDirect,
	}

	// Check if the exchange type is not direct
	isDirectNonDirect := amqpBrokerNonDirect.isDirectExchange()
	if isDirectNonDirect {
		t.Error("Expected exchange type not to be direct, got true")
	}
}

func TestReleaseConnection(t *testing.T) {
	// Prepare a sample AMQPBroker instance
	cfg := &config.Config{}
	amqpBroker := &AMQPBroker{
		Config:           cfg,
		connectionsMutex: sync.Mutex{},
	}

	// Create a sample connection
	sampleConnection := &amqp.Connection{}

	// Ensure the initial length of the connection pool is 0
	if len(amqpBroker.ConnectionPool) != 0 {
		t.Errorf("Expected initial connection pool length to be 0, got %d", len(amqpBroker.ConnectionPool))
	}

	// Call releaseConnection to add the sample connection to the pool
	amqpBroker.releaseConnectionToPool(sampleConnection)

	// Check if the connection pool length is now 1
	if len(amqpBroker.ConnectionPool) != 1 {
		t.Errorf("Expected connection pool length to be 1 after releasing a connection, got %d", len(amqpBroker.ConnectionPool))
	}

	// Check if the released connection is the same as the one added to the pool
	releasedConnection := amqpBroker.ConnectionPool[0]
	if releasedConnection != sampleConnection {
		t.Error("Expected released connection to be the same as the sample connection")
	}
}

func TestGetConnection(t *testing.T) {
	// Prepare a sample AMQPBroker instance
	cfg := &config.Config{}
	amqpBroker := &AMQPBroker{
		Config:           cfg,
		connectionsMutex: sync.Mutex{},
	}

	conn, err := amqpBroker.getConnection()
	// Check for errors
	assert.Error(t, err, "connection pool is empty")
	assert.Nil(t, conn, "Expected connection to be nil")

	// Create two sample connections
	sampleConnection1 := &amqp.Connection{}
	sampleConnection2 := &amqp.Connection{}

	// Set the initial connection pool
	amqpBroker.ConnectionPool = []*amqp.Connection{sampleConnection1, sampleConnection2}

	// Ensure the initial length of the connection pool is 2
	assert.Equal(t, 2, len(amqpBroker.ConnectionPool), "Initial connection pool length should be 2")

	// Call getConnection to get a connection from the pool
	conn, err = amqpBroker.getConnection()

	// Check for errors
	assert.NoError(t, err, "No error expected while getting a connection")
	assert.NotNil(t, conn, "Expected connection to be non-nil")

	// Check if the returned connection is the same as the first sample connection
	assert.Equal(t, sampleConnection1, conn, "Returned connection should be the first sample connection")

	// Check if the length of the connection pool is now 1
	assert.Equal(t, 1, len(amqpBroker.ConnectionPool), "Connection pool length should be 1 after getting a connection")

	// Call getConnection again to get another connection
	conn2, err2 := amqpBroker.getConnection()

	// Check for errors
	assert.NoError(t, err2, "No error expected while getting another connection")
	assert.NotNil(t, conn2, "Expected another connection to be non-nil")

	// Check if the returned connection is the same as the second sample connection
	assert.Equal(t, sampleConnection2, conn2, "Returned connection should be the second sample connection")

	// Check if the length of the connection pool is now 0
	assert.Equal(t, 0, len(amqpBroker.ConnectionPool), "Connection pool length should be 0 after getting another connection")
}

func TestCreateConnection(t *testing.T) {
	// Prepare a sample AMQPBroker instance
	cfg := &config.Config{
		AMQP: &config.AMQPConfig{
			Url: "sample-amqp-url",
		},
	}
	amqpBroker := &AMQPBroker{
		Config: cfg,
	}

	// Call createConnection to create a new connection
	conn, err := amqpBroker.createConnection()
	// Check for errors
	assert.Error(t, err, "AMQP scheme must be either 'amqp://' or 'amqps://'")
	assert.Nil(t, conn, "Expected connection to be nil")
}
