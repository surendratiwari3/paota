package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/schema"
)

// MockRedisConn now includes DoWithTimeout method
type MockRedisConn struct {
	mock.Mock
}

func (m *MockRedisConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisConn) Err() error {
	args := m.Called()
	return args.Error(0)
}

// Add this method to mock DoWithTimeout
func (m *MockRedisConn) DoWithTimeout(timeout time.Duration, commandName string, args ...interface{}) (interface{}, error) {
	// Combine timeout and all other arguments
	callArgs := make([]interface{}, 1+len(args)+1)
	callArgs[0] = timeout
	callArgs[1] = commandName
	copy(callArgs[2:], args)

	methodArgs := m.Called(callArgs...)
	return methodArgs.Get(0), methodArgs.Error(1)
}

func (m *MockRedisConn) Do(commandName string, args ...interface{}) (interface{}, error) {
	callArgs := make([]interface{}, 1+len(args))
	callArgs[0] = commandName
	copy(callArgs[1:], args)

	methodArgs := m.Called(callArgs...)
	return methodArgs.Get(0), methodArgs.Error(1)
}

// MockRedisPool remains the same
type MockRedisPool struct {
	mock.Mock
}

func (m *MockRedisPool) Get() redis.Conn {
	args := m.Called()
	conn, ok := args.Get(0).(redis.Conn)
	if !ok {
		return nil
	}
	return conn
}

func (m *MockRedisPool) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestPublish(t *testing.T) {
	testCases := []struct {
		name          string
		mockSetup     func(*MockRedisPool, *MockRedisConn)
		message       *schema.Signature
		queue         string
		expectedError bool
		errorMessage  string
	}{
		{
			name: "Successful Publish",
			mockSetup: func(pool *MockRedisPool, conn *MockRedisConn) {
				// Setup pool to return mock connection
				pool.On("Get").Return(conn)

				// Setup connection to expect Close() call
				conn.On("Close").Return(nil)

				// Prepare payload for LPUSH
				signature := &schema.Signature{Name: "test_task"}
				payload, _ := json.Marshal(signature)

				// Expect DoWithTimeout method with correct arguments
				conn.On("DoWithTimeout",
					mock.Anything, // timeout duration
					"LPUSH",
					"test_queue",
					payload,
				).Return(1, nil)
			},
			message:       &schema.Signature{Name: "test_task"},
			queue:         "test_queue",
			expectedError: false,
		},
		{
			name: "Failed Connection",
			mockSetup: func(pool *MockRedisPool, conn *MockRedisConn) {
				// Simulate nil connection
				pool.On("Get").Return(nil)
			},
			message:       &schema.Signature{Name: "test_task"},
			queue:         "test_queue",
			expectedError: true,
			errorMessage:  "failed to get connection from pool",
		},
		{
			name: "JSON Marshaling Error",
			mockSetup: func(pool *MockRedisPool, conn *MockRedisConn) {
				// Setup pool to return mock connection
				pool.On("Get").Return(conn)

				// Setup connection to expect Close() call
				conn.On("Close").Return(nil)
			},
			queue:         "test_queue",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockPool := new(MockRedisPool)
			mockConn := new(MockRedisConn)

			// Setup mocks for this specific test case
			tc.mockSetup(mockPool, mockConn)

			redisConfig := &config.RedisConfig{
				Address:      "localhost:6379",
				WriteTimeout: 5,
			}

			provider := &redisProvider{
				config: redisConfig,
				pool:   mockPool,
			}

			// Convert nil connection handling for "Failed Connection" case
			if mockPool.Get() == nil {
				err := provider.Publish(context.Background(), tc.queue, tc.message)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get connection from pool")
				return
			}

			err := provider.Publish(context.Background(), tc.queue, tc.message)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorMessage != "" {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockPool.AssertExpectations(t)
			mockConn.AssertExpectations(t)
		})
	}
}

func TestCloseConnection(t *testing.T) {
	mockPool := new(MockRedisPool)
	mockPool.On("Close").Return(nil)

	redisConfig := &config.RedisConfig{
		Address: "localhost:6379",
	}

	provider := &redisProvider{
		config: redisConfig,
		pool:   mockPool,
	}

	err := provider.CloseConnection()
	assert.NoError(t, err)
	mockPool.AssertExpectations(t)
}

func TestNewRedisProvider(t *testing.T) {
	redisConfig := &config.RedisConfig{
		Address:      "localhost:6379",
		MaxIdle:      10,
		IdleTimeout:  30,
		ReadTimeout:  5,
		WriteTimeout: 5,
	}

	provider := NewRedisProvider(redisConfig)
	assert.NotNil(t, provider)
}

func TestSubscribe(t *testing.T) {
	testCases := []struct {
		name          string
		setupMocks    func(*MockRedisPool, *MockRedisConn)
		queue         string
		handler       func(*schema.Signature) error
		expectedError bool
		errorContains string
	}{
		{
			name: "Empty Queue Name",
			setupMocks: func(pool *MockRedisPool, conn *MockRedisConn) {
				// No mock setup needed for this case
			},
			queue: "",
			handler: func(s *schema.Signature) error {
				return nil
			},
			expectedError: true,
			errorContains: "queue name cannot be empty",
		},
		{
			name: "Nil Handler Function",
			setupMocks: func(pool *MockRedisPool, conn *MockRedisConn) {
				// No mock setup needed for this case
			},
			queue:         "test_queue",
			handler:       nil,
			expectedError: true,
			errorContains: "handler function cannot be nil",
		},
		{
			name: "Successful Subscribe",
			setupMocks: func(pool *MockRedisPool, conn *MockRedisConn) {
				pool.On("Get").Return(conn)
				conn.On("Close").Return(nil)
				
				// Mock successful BRPOP response
				signature := &schema.Signature{Name: "test_task"}
				payload, _ := json.Marshal(signature)
				conn.On("DoWithTimeout", 
					mock.Anything,
					"BRPOP",
					"test_queue",
					mock.Anything,
				).Return([]interface{}{[]byte("test_queue"), payload}, nil)
			},
			queue: "test_queue",
			handler: func(s *schema.Signature) error {
				return nil
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockPool := new(MockRedisPool)
			mockConn := new(MockRedisConn)

			tc.setupMocks(mockPool, mockConn)

			redisConfig := &config.RedisConfig{
				Address:      "localhost:6379",
				ReadTimeout:  5,
				WriteTimeout: 5,
			}
			provider := &redisProvider{
				config: redisConfig,
				pool:   mockPool,
			}

			if provider.pool == nil {
				err := provider.Subscribe(tc.queue, tc.handler)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
				return
			}

			err := provider.Subscribe(tc.queue, tc.handler)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockPool.AssertExpectations(t)
			mockConn.AssertExpectations(t)
		})
	}
}

func TestValidateInputs(t *testing.T) {
	provider := &redisProvider{
		config: &config.RedisConfig{},
		pool:   new(MockRedisPool),
	}

	testCases := []struct {
		name          string
		queue         string
		handler       func(*schema.Signature) error
		expectedError string
	}{
		{
			name:          "Empty Queue",
			queue:         "",
			handler:       func(*schema.Signature) error { return nil },
			expectedError: "queue name cannot be empty",
		},
		{
			name:          "Nil Handler",
			queue:         "test_queue",
			handler:       nil,
			expectedError: "handler function cannot be nil",
		},
		{
			name:          "Valid Inputs",
			queue:         "test_queue",
			handler:       func(*schema.Signature) error { return nil },
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := provider.validateInputs(tc.queue, tc.handler)
			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
func TestGetConn(t *testing.T) {
	// Create mock pool and conn
	mockPool := new(MockRedisPool)
	mockConn := new(MockRedisConn)

	// Set up expectations
	mockPool.On("Get").Return(mockConn)

	// Create provider with mock pool
	provider := &redisProvider{
		config: &config.RedisConfig{},
		pool:   mockPool,
	}

	// Call GetConn
	 provider.GetConn()

	// Verify expectations
	mockPool.AssertExpectations(t)
	mockPool.AssertCalled(t, "Get")

	// Verify returned connection
	//assert.Equal(t, mockConn, conn, "Should return the mock connection")
}