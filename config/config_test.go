package config

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadFromEnv(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("PAOTA_BROKER", "amqp")

	// Test ReadFromEnv function
	err := GetConfigProvider().ReadFromEnv()
	assert.NoError(t, err)

	// Test GetConfig function after ReadFromEnv
	config := GetConfigProvider().GetConfig()
	assert.NotNil(t, config)
}

func TestGetConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("PAOTA_BROKER", "amqp")
	os.Setenv("PAOTA_STORE", "mongodb")
	os.Setenv("PAOTA_QUEUE_NAME", "test_queue")
	os.Setenv("PAOTA_STORE_QUEUE_NAME", "test_store_queue")

	// Ensure environment variables are cleaned up after the test
	defer func() {
		os.Unsetenv("PAOTA_BROKER")
		os.Unsetenv("PAOTA_STORE")
		os.Unsetenv("PAOTA_QUEUE_NAME")
		os.Unsetenv("PAOTA_STORE_QUEUE_NAME")
	}()

	err := GetConfigProvider().ReadFromEnv()
	assert.NoError(t, err)

	// Test GetConfig function
	config := GetConfigProvider().GetConfig()

	// Assert that the returned config is not nil
	assert.NotNil(t, config)

	// Assert that the config values match the expected values
	assert.Equal(t, "amqp", config.Broker)
	assert.Equal(t, "mongodb", config.Store)
	assert.Equal(t, "test_queue", config.TaskQueueName)
	assert.Equal(t, "test_store_queue", config.StoreQueueName)
}

func TestEmptyGetConfig(t *testing.T) {
	SetConfigProvider(NewConfigProvider())

	// Test GetConfig function
	config := GetConfigProvider().GetConfig()

	// Assert that the returned config is nil
	assert.Nil(t, config)
}

func TestValidateConfig_InvalidConfig(t *testing.T) {
	// Create an invalid configuration for testing (missing required field)
	invalidConfig := Config{
		Broker: "amqp1",
	}

	// Test ValidateConfig function with an invalid configuration
	err := GetConfigProvider().ValidateConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
	assert.Contains(t, err.Error(), "oneof")
}

func TestValidateConfig_validConfig(t *testing.T) {
	// Create an invalid configuration for testing (missing required field)
	invalidConfig := Config{
		Broker:        "amqp",
		TaskQueueName: "test",
	}

	// Test ValidateConfig function with an invalid configuration
	err := GetConfigProvider().ValidateConfig(invalidConfig)
	assert.Nil(t, err)
}

func TestSetApplicationConfig_invalidConfig(t *testing.T) {
	invalidConfig := Config{
		Broker:        "test",
		TaskQueueName: "test",
	}
	err := GetConfigProvider().SetApplicationConfig(invalidConfig)
	assert.NotNil(t, err)
}

func TestSetApplicationConfig_validConfig(t *testing.T) {
	invalidConfig := Config{
		Broker:        "amqp",
		TaskQueueName: "test",
	}
	err := GetConfigProvider().SetApplicationConfig(invalidConfig)
	assert.Nil(t, err)
}
