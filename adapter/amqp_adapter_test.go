package adapter

import (
	"github.com/stretchr/testify/assert"
	"github.com/surendratiwari3/paota/config"
	"testing"
)

// TestAMQPAdapter tests the functionality of the AMQPAdapter
func TestAMQPAdapter(t *testing.T) {
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
