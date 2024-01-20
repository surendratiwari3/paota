package amqp

import (
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
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
