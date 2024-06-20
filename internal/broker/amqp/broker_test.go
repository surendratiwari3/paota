package amqp

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/provider"
)

func TestNewAMQPBroker(t *testing.T) {

	mockAmqpProvider := new(provider.MockAmqpProviderInterface)
	mockConfigProvider := new(config.MockConfigProvider)

	conf := &config.Config{
		Broker:        "amqp",
		TaskQueueName: "test",
		AMQP: &config.AMQPConfig{
			Exchange:           "amqp",
			ExchangeType:       "direct",
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 2,
			DelayedQueue:       "test",
		},
	}

	mockConfigProvider.On("GetConfig").Return(conf, nil)

	conn := &amqp.Connection{}
	channel := &amqp.Channel{}
	exchangeName := conf.AMQP.Exchange
	exchangeType := conf.AMQP.ExchangeType

	mockAmqpProvider.On("CreateConnectionPool").Return(nil)
	mockAmqpProvider.On("GetConnectionFromPool").Return(conn, nil)
	mockAmqpProvider.On("ReleaseConnectionToPool", conn).Return(nil)
	mockAmqpProvider.On("CreateAmqpChannel", conn, false).Return(channel, nil, nil)
	mockAmqpProvider.On("DeclareExchange", channel, exchangeName, exchangeType).Return(nil)
	mockAmqpProvider.On("DeclareQueue", channel, mock.Anything, mock.Anything).Return(nil)
	mockAmqpProvider.On("QueueExchangeBind", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockAmqpProvider.On("CloseAmqpChannel", mock.Anything).Return(nil)

	config.SetConfigProvider(mockConfigProvider)
	globalAmqpProvider = mockAmqpProvider

	// Create a new instance of AMQPBroker
	broker, err := NewAMQPBroker("master")

	// Perform assertions as needed
	assert.Nil(t, err)
	assert.NotNil(t, broker)
}

func TestAMQPBrokerGetRoutingKey(t *testing.T) {
	cfg := &config.Config{
		Broker:        "amqp",
		TaskQueueName: "test_queue",
		AMQP: &config.AMQPConfig{
			Exchange:           "exchange",
			ExchangeType:       "fanout",
			BindingKey:         "test_key",
			Url:                "amqp://localhost:5672",
			HeartBeatInterval:  30,
			ConnectionPoolSize: 2,
			DelayedQueue:       "delay_queue",
			PrefetchCount:      100,
		},
	}

	broker := &AMQPBroker{
		config: cfg.AMQP,
	}

	require.Equal(t, "test_queue", broker.getTaskQueue(), "TaskQueueName should match")
	require.Equal(t, "test_queue", broker.getRoutingKey(), "Routing key should match the direct exchange binding key")
	require.Equal(t, "fanout", broker.getExchangeType(), "exchange name key should match")
	require.Equal(t, "delay_queue", broker.getDelayedQueue(), "delay_queue name key should match")
	require.Equal(t, 100, broker.getQueuePrefetchCount(), "prefetch count key should match")
	require.Equal(t, "exchange", broker.getExchangeName(), "Exchange key should match")
	require.Equal(t, "exchange", broker.getDelayedQueueDLX(), "Exchange key should match")
}

func TestIsDirectExchange(t *testing.T) {
	// Prepare a sample AMQPBroker instance with a direct exchange
	cfg := &config.Config{
		AMQP: &config.AMQPConfig{
			ExchangeType: "direct",
		},
	}
	amqpBroker := &AMQPBroker{
		config: cfg.AMQP,
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
		config: cfgNonDirect.AMQP,
	}

	// Check if the exchange type is not direct
	isDirectNonDirect := amqpBrokerNonDirect.isDirectExchange()
	if isDirectNonDirect {
		t.Error("Expected exchange type not to be direct, got true")
	}
}
