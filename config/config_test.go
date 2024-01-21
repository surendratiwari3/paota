package config

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadFromEnv(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("PAOTA_BROKER", "test_broker")

	// Test ReadFromEnv function
	err := ReadFromEnv()
	assert.NoError(t, err)

	// Test GetConfig function after ReadFromEnv
	config := GetConfig()
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

	err := ReadFromEnv()
	assert.NoError(t, err)

	// Test GetConfig function
	config := GetConfig()

	// Assert that the returned config is not nil
	assert.NotNil(t, config)

	// Assert that the config values match the expected values
	assert.Equal(t, "amqp", config.Broker)
	assert.Equal(t, "mongodb", config.Store)
	assert.Equal(t, "test_queue", config.TaskQueueName)
	assert.Equal(t, "test_store_queue", config.StoreQueueName)
}
