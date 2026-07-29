package config

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetApplicationConfigAppliesExecutionDurationDefault(t *testing.T) {
	provider := NewConfigProvider()

	require.NoError(t, provider.SetApplicationConfig(Config{
		Broker:        "amqp",
		TaskQueueName: "tasks",
	}))

	cfg := provider.GetConfig()
	require.NotNil(t, cfg)
	// A zero here would give every task an already-expired context.
	assert.Equal(t, 10, cfg.MaxTaskExecutionDuration)
}

func TestSetApplicationConfigKeepsExplicitExecutionDuration(t *testing.T) {
	provider := NewConfigProvider()

	require.NoError(t, provider.SetApplicationConfig(Config{
		Broker:                   "amqp",
		TaskQueueName:            "tasks",
		MaxTaskExecutionDuration: 45,
	}))

	assert.Equal(t, 45, provider.GetConfig().MaxTaskExecutionDuration)
}

func TestSetApplicationConfigRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"unsupported broker", Config{Broker: "kafka", TaskQueueName: "tasks"}},
		{"missing broker", Config{TaskQueueName: "tasks"}},
		{"missing queue name", Config{Broker: "amqp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewConfigProvider()
			assert.Error(t, provider.SetApplicationConfig(tt.cfg))
			// A rejected config must not be stored.
			assert.Nil(t, provider.GetConfig())
		})
	}
}

func TestGetConfigReturnsNilBeforeConfiguration(t *testing.T) {
	provider := NewConfigProvider()
	assert.Nil(t, provider.GetConfig())
}

func TestGetConfigProviderIsLazilyInitialised(t *testing.T) {
	SetConfigProvider(nil)
	provider := GetConfigProvider()

	assert.NotNil(t, provider)
	// The global is memoised, so a second call returns the same instance.
	assert.Same(t, provider, GetConfigProvider())
}

func TestSetConfigProviderOverridesGlobal(t *testing.T) {
	custom := NewConfigProvider()
	require.NoError(t, custom.SetApplicationConfig(Config{
		Broker:        "amqp",
		TaskQueueName: "custom_queue",
	}))

	SetConfigProvider(custom)
	t.Cleanup(func() { SetConfigProvider(nil) })

	assert.Equal(t, "custom_queue", GetConfigProvider().GetConfig().TaskQueueName)
}

// TestConcurrentConfigReads runs under -race to check that a config shared by
// every worker goroutine is only ever read after construction.
func TestConcurrentConfigReads(t *testing.T) {
	provider := NewConfigProvider()
	require.NoError(t, provider.SetApplicationConfig(Config{
		Broker:        "amqp",
		TaskQueueName: "tasks",
	}))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := provider.GetConfig()
			assert.Equal(t, "tasks", cfg.TaskQueueName)
		}()
	}
	wg.Wait()
}
