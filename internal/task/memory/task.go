package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/broker/redis"
	"github.com/surendratiwari3/paota/internal/task"

	"sync"
	"time"

	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/utils"
	"github.com/surendratiwari3/paota/internal/validation"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	appError "github.com/surendratiwari3/paota/schema/errors"
)

type DefaultTaskRegistrar struct {
	registeredTasks      *sync.Map
	registeredTasksCount uint
	broker               broker.Broker
	configProvider       config.ConfigProvider
}

func NewDefaultTaskRegistrar(brk broker.Broker, configProvider config.ConfigProvider) task.TaskRegistrarInterface {
	return &DefaultTaskRegistrar{
		registeredTasks: new(sync.Map),
		broker:          brk,
		configProvider:  configProvider,
	}
}

// RegisterTasks registers all tasks at once
func (r *DefaultTaskRegistrar) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	for _, taskFunc := range namedTaskFuncs {
		if err := validation.ValidateTask(taskFunc); err != nil {
			return err
		}
	}

	for k, v := range namedTaskFuncs {
		r.registeredTasksCount = r.registeredTasksCount + 1
		r.registeredTasks.Store(k, v)
	}
	return nil
}

//  priority,
//  routing_key
// 	Priority                    uint8
//	RetryCount                  int
//	RetryTimeout                int

func (r *DefaultTaskRegistrar) RegisterTasksWithOptions(namedTaskFuncs map[string]interface{}) error {
	return nil
}

// IsTaskRegistered returns true if the task name is registered with this broker
func (r *DefaultTaskRegistrar) IsTaskRegistered(name string) bool {
	_, ok := r.registeredTasks.Load(name)
	return ok
}

// GetRegisteredTask returns registered task by name
func (r *DefaultTaskRegistrar) GetRegisteredTask(name string) (interface{}, error) {
	taskFunc, ok := r.registeredTasks.Load(name)
	if !ok {
		return nil, fmt.Errorf("task not registered error: %s", name)
	}
	return taskFunc, nil
}

func (r *DefaultTaskRegistrar) GetRegisteredTaskCount() uint {
	return r.registeredTasksCount
}

func (r *DefaultTaskRegistrar) Processor(job interface{}) error {
	logger.ApplicationLogger.Info("Received job of type", "type", fmt.Sprintf("%T", job))

	// Perform a type switch to handle different job types
	switch j := job.(type) {
	case amqp.Delivery:
		return r.amqpMsgProcessor(j) // Pass the job as amqp.Delivery
	case *schema.Signature:
		// Perform a type assertion to ensure job is of type *schema.Signature
		return r.redisMsgProcessor(j) // Use j directly since it's already asserted
	default:
		// Handle other job types or report an error
		logger.ApplicationLogger.Error("Unsupported job type", "type", fmt.Sprintf("%T", j), "job", j)
	}
	return nil
}

func (r *DefaultTaskRegistrar) amqpMsgProcessor(job interface{}) error {
	multiple := false
	requeue := false
	if amqpJob, ok := job.(amqp.Delivery); !ok {
		return errors.ErrUnsupported
	} else if len(amqpJob.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		err := amqpJob.Nack(false, false)
		if err != nil {
			logger.ApplicationLogger.Error("error while sending nack for message")
		} // multiple, requeue
		return appError.ErrEmptyMessage // RabbitMQ down?
	} else if signature, err := schema.BytesToSignature(amqpJob.Body); err != nil {
		logger.ApplicationLogger.Error("decode error in message, return")
		err := amqpJob.Nack(false, false)
		if err != nil {
			return err
		}
		return err
	} else if registeredTaskFunc, err := r.GetRegisteredTask(signature.Name); err != nil {
		logger.ApplicationLogger.Error("invalid task function")
		amqpJob.Nack(multiple, requeue)
		return appError.ErrTaskMustBeFunc
	} else if registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("task is not registered")
		amqpJob.Nack(multiple, requeue)
		return appError.ErrTaskNotRegistered
	} else if fn, ok := registeredTaskFunc.(func(*schema.Signature) error); !ok {
		return amqpJob.Nack(multiple, requeue)
	} else if err = fn(signature); err == nil {
		err := amqpJob.Ack(false)
		if err != nil {
			return err
		}
	} else if err = r.retryTask(signature); err != nil {
		requeue = true
		return amqpJob.Nack(multiple, requeue)
	} else {
		return amqpJob.Ack(false)
	}
	return nil
}

func (r *DefaultTaskRegistrar) redisMsgProcessor(signature *schema.Signature) error {
	registeredTaskFunc, err := r.GetRegisteredTask(signature.Name)
	if err != nil || registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("task not registered or invalid task function", signature.Name)
		return appError.ErrTaskNotRegistered
	}

	fn, ok := registeredTaskFunc.(func(*schema.Signature) error)
	if !ok {
		logger.ApplicationLogger.Error("Invalid task function type", signature.Name)
		return appError.ErrTaskMustBeFunc
	}

	// Execute the task
	if err := fn(signature); err != nil {
		// If task execution fails, handle retry logic
		if err := r.retryRedisTask(signature); err != nil {
			logger.ApplicationLogger.Error("Failed to retry task", "error", err)
			return err
		}
		return err
	}

	return nil
}

func (r *DefaultTaskRegistrar) retryRedisTask(signature *schema.Signature) error {
	// Set default retry values if not specified
	if signature.RetryCount == 0 {
		signature.RetryCount = r.configProvider.GetConfig().Redis.RetryCount
	}
	if signature.RetryTimeout == 0 {
		signature.RetryTimeout = r.configProvider.GetConfig().Redis.RetryTimeout
	}

	// Check if we've exceeded retry count
	if signature.RetriesDone >= signature.RetryCount {
		logger.ApplicationLogger.Info("Max retry attempts reached",
			"task", signature.Name,
			"attempts", signature.RetriesDone,
		)
		return fmt.Errorf("max retry attempts reached for task: %s", signature.Name)
	}

	// Increment retry counter
	signature.RetriesDone++

	// Calculate next retry time using the signature's RetryTimeout
	retryDelay := time.Duration(signature.RetryTimeout) * time.Second
	eta := time.Now().UTC().Add(retryDelay)
	signature.ETA = &eta

	if redisBroker, ok := r.broker.(*redis.RedisBroker); ok {
		delayedKey := r.configProvider.GetConfig().Redis.DelayedTasksKey
		if delayedKey == "" {
			delayedKey = "paota_tasks_delayed"
		}

		taskBytes, err := json.Marshal(signature)
		if err != nil {
			return fmt.Errorf("failed to serialize task: %v", err)
		}

		conn := redisBroker.GetProvider().GetConn()
		defer conn.Close()

		// Use ETA's Unix timestamp as score for proper delay
		_, err = conn.Do("ZADD", delayedKey, eta.Unix(), taskBytes)
		if err != nil {
			return fmt.Errorf("failed to add task to delayed queue: %v", err)
		}

		logger.ApplicationLogger.Info("Task scheduled for retry",
			"task", signature.Name,
			"attempt", signature.RetriesDone,
			"nextRetry", eta,
			"delaySeconds", signature.RetryTimeout)

		return nil
	}

	return fmt.Errorf("broker is not of type RedisBroker")
}

func (r *DefaultTaskRegistrar) retryTask(signature *schema.Signature) error {
	if signature.RetryCount < 1 {
		return nil
	}
	signature.RetriesDone = signature.RetriesDone + 1
	if signature.RetriesDone > signature.RetryCount {
		return nil
	}
	retryInterval := r.getRetryInterval(signature.RetriesDone)
	if retryInterval > 0 {
		signature.RoutingKey = r.configProvider.GetConfig().AMQP.DelayedQueue
	}
	eta := time.Now().UTC().Add(retryInterval)
	signature.ETA = &eta
	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) getRetryInterval(retryCount int) time.Duration {
	return time.Duration(utils.Fibonacci(retryCount)) * time.Second
}

func (r *DefaultTaskRegistrar) SendTask(signature *schema.Signature) error {
	return r.SendTaskWithContext(context.Background(), signature)
}

func (r *DefaultTaskRegistrar) SendTaskWithContext(ctx context.Context, signature *schema.Signature) error {
	// Auto generate a UUID if not set already
	if signature.UUID == "" {
		taskID := uuid.New().String()
		signature.UUID = fmt.Sprintf("task_%v", taskID)
	}

	// Use BrokerType to determine the broker and send the task accordingly
	switch r.broker.BrokerType() {
	case "rabbitmq":
		// If the broker is RabbitMQ, publish to the message queue
		if err := r.broker.Publish(ctx, signature); err != nil {
			return fmt.Errorf("publish message to RabbitMQ error: %s", err)
		}
		logger.ApplicationLogger.Info("Task published to RabbitMQ", "taskUUID", signature.UUID)
	case "redis":
		// If the broker is Redis, pass the signature directly to RedisBroker's Publish method
		if redisBroker, ok := r.broker.(*redis.RedisBroker); ok {
			// Publish directly without serialization because RedisBroker's Publish already handles it
			if err := redisBroker.Publish(ctx, signature); err != nil {
				return fmt.Errorf("failed to store task in Redis: %s", err)
			}
			logger.ApplicationLogger.Info("Task published to Redis", "taskUUID", signature.UUID)
		} else {
			return fmt.Errorf("broker is not of type RedisBroker")
		}

	default:
		// If the broker type is unsupported
		return fmt.Errorf("unsupported broker type: %s", r.broker.BrokerType())
	}

	return nil
}
func SignatureToBytes(signature *schema.Signature) ([]byte, error) {
	// Serialize signature to JSON (you can use other formats if needed)
	return json.Marshal(signature)
}
