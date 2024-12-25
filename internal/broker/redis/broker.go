package redis

import (
	"context"
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

func (rb *RedisBroker) Publish(ctx context.Context, signature *schema.Signature) error {
	return rb.provider.Publish(ctx, rb.config.TaskQueueName, signature)
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
			if delayedKey == "" {
				delayedKey = "paota_tasks_delayed" // Ensure default key
			}
			mainQueue := rb.config.TaskQueueName

			conn := rb.provider.GetConn()
			now := time.Now().UTC().Unix()

			logger.ApplicationLogger.Debug("Checking delayed tasks",
				"delayedKey", delayedKey,
				"currentTime", now)

			// Get tasks that are ready (score <= current timestamp)
			tasks, err := redis.Values(conn.Do("ZRANGEBYSCORE", delayedKey, "-inf", now))
			if err != nil {
				logger.ApplicationLogger.Error("Failed to get delayed tasks",
					"error", err,
					"delayedKey", delayedKey)
				conn.Close()
				continue
			}

			logger.ApplicationLogger.Debug("Found delayed tasks", "count", len(tasks))

			for _, task := range tasks {
				taskBytes, ok := task.([]byte)
				if !ok {
					logger.ApplicationLogger.Error("Invalid task data type")
					continue
				}

				// Log task details before moving
				logger.ApplicationLogger.Info("Moving task to main queue",
					"mainQueue", mainQueue,
					"taskData", string(taskBytes))

				// Move to main queue and remove from delayed set atomically
				conn.Send("MULTI")
				conn.Send("LPUSH", mainQueue, taskBytes)
				conn.Send("ZREM", delayedKey, taskBytes)
				reply, err := redis.Values(conn.Do("EXEC"))

				if err != nil {
					logger.ApplicationLogger.Error("Failed to move delayed task",
						"error", err,
						"mainQueue", mainQueue)
				} else {
					logger.ApplicationLogger.Info("Successfully moved delayed task",
						"mainQueue", mainQueue,
						"reply", reply)
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
