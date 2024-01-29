package amqp

import (
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/config"
	"testing"
)

func TestNewAMQPBroker(t *testing.T) {

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

	// Create a new instance of AMQPBroker
	broker, err := NewAMQPBroker()

	// Perform assertions as needed
	assert.Error(t, err)
	assert.Nil(t, broker)
}

/*
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
*/
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
