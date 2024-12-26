package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
)

type RedisBroker struct {
	provider provider.RedisProviderInterface
	config   *config.Config
	stopChan chan struct{}
	running  bool
}

func NewRedisBroker(provider provider.RedisProviderInterface, config *config.Config) (broker.Broker, error) {
	broker := &RedisBroker{
		provider: provider,
		config:   config,
		stopChan: make(chan struct{}),
	}

	broker.running = true
	go broker.processDelayedTasks()

	logger.ApplicationLogger.Info("Started Redis delayed task processor")
	return broker, nil
}

func (rb *RedisBroker) getTaskDelay(signature *schema.Signature) time.Duration {
	if signature.ETA != nil {
		now := time.Now().UTC()
		if signature.ETA.After(now) {
			return signature.ETA.Sub(now)
		}
	}
	return 0
}

func (rb *RedisBroker) Publish(ctx context.Context, signature *schema.Signature) error {
	delay := rb.getTaskDelay(signature)

	if delay > 0 {
		// Add to delayed queue with future timestamp as score
		taskBytes, err := json.Marshal(signature)
		if err != nil {
			return fmt.Errorf("failed to marshal delayed task: %v", err)
		}

		conn := rb.provider.GetConn()
		defer conn.Close()

		// Calculate exact execution time
		executeAt := time.Now().UTC().Add(delay).Unix()

		_, err = conn.Do("ZADD", rb.config.Redis.DelayedTasksKey, executeAt, taskBytes)
		if err != nil {
			return fmt.Errorf("failed to add delayed task: %v", err)
		}

		logger.ApplicationLogger.Info("Task scheduled for delayed execution",
			"task", signature.Name,
			"delay", delay,
			"executeAt", time.Unix(executeAt, 0))
		return nil
	}

	// No delay, publish immediately
	return rb.publishToMainQueue(ctx, signature)
}

func (rb *RedisBroker) StartConsumer(ctx context.Context, workerGroup workergroup.WorkerGroupInterface) error {
	return rb.provider.Subscribe(rb.config.TaskQueueName, func(signature *schema.Signature) error {
		workerGroup.AssignJob(signature)
		return nil
	})
}

func (rb *RedisBroker) StopConsumer() {
	_ = rb.provider.CloseConnection()
}

func (rb *RedisBroker) BrokerType() string {
	return "redis" // Return "redis" to indicate it's a Redis broker
}

func (rb *RedisBroker) LPush(ctx context.Context, key string, value interface{}) error {
	conn := rb.provider.GetConn()
	defer conn.Close()
	_, err := conn.Do("LPUSH", key, value)
	return err
}

func (rb *RedisBroker) BLPop(ctx context.Context, key string) (interface{}, error) {
	conn := rb.provider.GetConn()
	defer conn.Close()
	return conn.Do("BLPOP", key, 0) // 0 means block indefinitely
}

func (rb *RedisBroker) StartDelayedTasksProcessor() {
	rb.stopChan = make(chan struct{})
	go rb.processDelayedTasks()
}

func (rb *RedisBroker) processDelayedTasks() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rb.stopChan:
			return
		case <-ticker.C:
			delayedKey := rb.config.Redis.DelayedTasksKey
			mainQueue := rb.config.TaskQueueName

			conn := rb.provider.GetConn()
			now := time.Now().UTC().Unix()

			// Get tasks that are ready (score <= current timestamp)
			tasks, err := redis.Values(conn.Do("ZRANGEBYSCORE", delayedKey, "-inf", now))
			if err != nil {
				logger.ApplicationLogger.Error("Failed to get delayed tasks", "error", err)
				conn.Close()
				continue
			}

			for _, task := range tasks {
				taskBytes, ok := task.([]byte)
				if !ok {
					continue
				}

				// Execute in a transaction to ensure atomicity
				conn.Send("MULTI")
				// Move to main queue
				conn.Send("LPUSH", mainQueue, taskBytes)
				// Remove from delayed queue
				conn.Send("ZREM", delayedKey, taskBytes)

				if _, err := redis.Values(conn.Do("EXEC")); err != nil {
					logger.ApplicationLogger.Error("Failed to move task to main queue",
						"error", err,
						"task", string(taskBytes))
				} else {
					logger.ApplicationLogger.Info("Moved delayed task to main queue",
						"task", string(taskBytes))
				}
			}
			conn.Close()
		}
	}
}

func (rb *RedisBroker) Stop() error {
	if rb.stopChan != nil {
		close(rb.stopChan)
	}
	return rb.provider.CloseConnection()
}

func (rb *RedisBroker) GetProvider() provider.RedisProviderInterface {
	return rb.provider
}

func (rb *RedisBroker) Close() error {
	if rb.running {
		rb.running = false
		close(rb.stopChan)
	}
	return nil
}

func (rb *RedisBroker) publishToMainQueue(ctx context.Context, signature *schema.Signature) error {
	taskBytes, err := json.Marshal(signature)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %v", err)
	}

	conn := rb.provider.GetConn()
	defer conn.Close()

	_, err = conn.Do("LPUSH", rb.config.TaskQueueName, taskBytes)
	return err
}
