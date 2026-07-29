package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeoutQueueHasItsOwnDefault covers a copy-paste bug: TimeoutQueue
// defaulted to the failed queue name, so tasks that ran out of time were mixed
// in with tasks that had exhausted their retries and could not be told apart.
func TestTimeoutQueueHasItsOwnDefault(t *testing.T) {
	t.Setenv("PAOTA_BROKER", "amqp")
	t.Setenv("PAOTA_AMQP_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("PAOTA_AMQP_PREFETCH_COUNT", "10")

	provider := NewConfigProvider()
	require.NoError(t, provider.ReadFromEnv())

	cfg := provider.GetConfig()
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.AMQP)

	assert.Equal(t, "paota_task_failed_queue", cfg.AMQP.FailedQueue)
	assert.Equal(t, "paota_task_timeout_queue", cfg.AMQP.TimeoutQueue)
	assert.NotEqual(t, cfg.AMQP.FailedQueue, cfg.AMQP.TimeoutQueue)
}

func TestAMQPDefaults(t *testing.T) {
	t.Setenv("PAOTA_BROKER", "amqp")
	t.Setenv("PAOTA_AMQP_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("PAOTA_AMQP_PREFETCH_COUNT", "10")

	provider := NewConfigProvider()
	require.NoError(t, provider.ReadFromEnv())
	cfg := provider.GetConfig().AMQP
	require.NotNil(t, cfg)

	assert.Equal(t, "paota_task_exchange", cfg.Exchange)
	assert.Equal(t, "direct", cfg.ExchangeType)
	assert.Equal(t, "paota_task_binding_key", cfg.BindingKey)
	assert.Equal(t, "paota_task_delayed_queue", cfg.DelayedQueue)
	assert.Equal(t, 5, cfg.ConnectionPoolSize)
	assert.Equal(t, 10, cfg.HeartBeatInterval)
	assert.Equal(t, 5, cfg.ConnectionTimeout)
	assert.Equal(t, 5, cfg.PublishConfirmTimeout)
	assert.Equal(t, 10, cfg.PrefetchCount)
}

func TestAMQPEnvOverrides(t *testing.T) {
	envs := map[string]string{
		"PAOTA_BROKER":                       "amqp",
		"PAOTA_AMQP_URL":                     "amqp://user:pass@rabbit:5672/vhost",
		"PAOTA_AMQP_PREFETCH_COUNT":          "42",
		"PAOTA_AMQP_EXCHANGE":                "custom_exchange",
		"PAOTA_AMQP_EXCHANGE_TYPE":           "topic",
		"PAOTA_AMQP_BINDING_KEY":             "custom_key",
		"PAOTA_AMQP_DELAYED_QUEUE":           "custom_delayed",
		"PAOTA_AMQP_FAILED_QUEUE":            "custom_failed",
		"PAOTA_AMQP_TIMEOUT_QUEUE":           "custom_timeout",
		"PAOTA_AMQP_CONNECTION_POOL_SIZE":    "3",
		"PAOTA_AMQP_HEARTBEAT_INTERVAL":      "15",
		"PAOTA_AMQP_CONNECTION_TIMEOUT":      "7",
		"PAOTA_AMQP_PUBLISH_CONFIRM_TIMEOUT": "11",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	provider := NewConfigProvider()
	require.NoError(t, provider.ReadFromEnv())
	cfg := provider.GetConfig().AMQP
	require.NotNil(t, cfg)

	assert.Equal(t, "amqp://user:pass@rabbit:5672/vhost", cfg.Url)
	assert.Equal(t, 42, cfg.PrefetchCount)
	assert.Equal(t, "custom_exchange", cfg.Exchange)
	assert.Equal(t, "topic", cfg.ExchangeType)
	assert.Equal(t, "custom_key", cfg.BindingKey)
	assert.Equal(t, "custom_delayed", cfg.DelayedQueue)
	assert.Equal(t, "custom_failed", cfg.FailedQueue)
	assert.Equal(t, "custom_timeout", cfg.TimeoutQueue)
	assert.Equal(t, 3, cfg.ConnectionPoolSize)
	assert.Equal(t, 15, cfg.HeartBeatInterval)
	assert.Equal(t, 7, cfg.ConnectionTimeout)
	assert.Equal(t, 11, cfg.PublishConfirmTimeout)
}

func TestAMQPUrlIsRequired(t *testing.T) {
	// The URL carries the credentials, so an AMQP block without one has to be
	// rejected rather than silently dialling an empty address.
	provider := NewConfigProvider()
	err := provider.ValidateConfig(Config{
		Broker:        "amqp",
		TaskQueueName: "tasks",
		AMQP:          &AMQPConfig{PrefetchCount: 1},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Url")
}

func TestMain(m *testing.M) {
	// Every test in this package reads PAOTA_* variables; make sure none leak
	// in from the developer's shell.
	for _, key := range os.Environ() {
		if len(key) > 6 && key[:6] == "PAOTA_" {
			os.Unsetenv(key)
		}
	}
	os.Exit(m.Run())
}
