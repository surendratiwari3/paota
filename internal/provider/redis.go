package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
)

type RedisPoolInterface interface {
	Get() redis.Conn
	Close() error
}
type RedisProviderInterface interface {
	Publish(ctx context.Context, queue string, message *schema.Signature) error
	Subscribe(queue string, handler func(*schema.Signature) error) error
	CloseConnection() error
}

type redisProvider struct {
	config *config.RedisConfig
	pool   RedisPoolInterface // Use the interface here
}

// NewRedisProvider creates a new Redis provider.
func NewRedisProvider(redisConfig *config.RedisConfig) RedisProviderInterface {
	return &redisProvider{
		config: redisConfig,
		pool: &redis.Pool{
			MaxIdle:     redisConfig.MaxIdle,
			IdleTimeout: time.Duration(redisConfig.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", redisConfig.Address)
			},
		},
	}
}

func (rp *redisProvider) Publish(ctx context.Context, queue string, message *schema.Signature) error {
	conn := rp.pool.Get()
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	if conn == nil {
		return fmt.Errorf("failed to get connection from pool")
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = redis.DoWithTimeout(conn, time.Duration(rp.config.WriteTimeout)*time.Second, "LPUSH", queue, payload)
	return err
}

func (rp *redisProvider) Subscribe(queue string, handler func(*schema.Signature) error) error {

	// Validate inputs early
	if err := rp.validateInputs(queue, handler); err != nil {
		return err
	}
	conn := rp.pool.Get()
	if conn == nil {
		return errors.New("failed to get connection from pool")
	}
	defer conn.Close()

	for {
		// Execute BRPOP with timeout
		payload, err := redis.Strings(redis.DoWithTimeout(conn, time.Duration(rp.config.ReadTimeout)*time.Second, "BRPOP", queue, rp.config.ReadTimeout))
		if err != nil {
			if err == redis.ErrNil {
				// BRPOP timeout: Log a warning and retry
				logger.ApplicationLogger.Warn("BRPOP timed out, no items in queue", "queue", queue)
				continue
			}
			// Log other errors and return to allow caller to handle
			logger.ApplicationLogger.Error("Error during BRPOP", "queue", queue, "error", err)
			return err
		}

		// Ensure payload has the correct structure
		if len(payload) < 2 {
			logger.ApplicationLogger.Warn("Received invalid payload from Redis", "payload", payload)
			continue
		}

		var signature schema.Signature
		if err := json.Unmarshal([]byte(payload[1]), &signature); err != nil {
			logger.ApplicationLogger.Error("Failed to unmarshal payload", "payload", payload[1], "error", err)
			continue
		}

		// Process the task using the handler
		if err := handler(&signature); err != nil {
			logger.ApplicationLogger.Error("Task handler returned an error", "signature", signature, "error", err)
			return err
		}
	}
}

func (rp *redisProvider) CloseConnection() error {
	return rp.pool.Close()
}

// validateInputs performs the necessary input validation
func (rp *redisProvider) validateInputs(queue string, handler func(*schema.Signature) error) error {
	if queue == "" {
		return errors.New("queue name cannot be empty")
	}
	if handler == nil {
		return errors.New("handler function cannot be nil")
	}
	if rp.pool == nil {
		return errors.New("redis pool is not initialized")
	}
	return nil
}
