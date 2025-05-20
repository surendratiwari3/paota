package memory

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/task"

	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/utils"
	"github.com/surendratiwari3/paota/internal/validation"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	appError "github.com/surendratiwari3/paota/schema/errors"
	"sync"
	"time"
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
		return nil, fmt.Errorf("Task not registered error: %s", name)
	}
	return taskFunc, nil
}

func (r *DefaultTaskRegistrar) GetRegisteredTaskCount() uint {
	return r.registeredTasksCount
}

func (r *DefaultTaskRegistrar) Processor(job interface{}) error {

	// Perform a type switch to handle different job types
	switch j := job.(type) {
	case amqp.Delivery:
		return r.amqpMsgProcessor(job)
	default:
		// Handle other job types or report an error
		logger.ApplicationLogger.Error("Unsupported job type", j)
	}
	return nil
}

func (r *DefaultTaskRegistrar) amqpMsgProcessor(job interface{}) error {
	if amqpJob, ok := job.(amqp.Delivery); !ok {
		return errors.ErrUnsupported
	} else if len(amqpJob.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		_ = amqpJob.Nack(false, false)
		return appError.ErrEmptyMessage
	} else if signature, err := schema.BytesToSignature(amqpJob.Body); err != nil {
		logger.ApplicationLogger.Error("decode error in message, return")
		_ = amqpJob.Nack(false, false)
		return err
	} else {
		return r.amqpProcessSignature(amqpJob, signature)
	}
}

func (r *DefaultTaskRegistrar) amqpProcessSignature(amqpJob amqp.Delivery, signature *schema.Signature) error {
	multiple := false
	requeue := false

	// Step 1: Timeout check
	if r.checkTaskTimeout(signature) {
		logger.ApplicationLogger.Warnf("Task %s timed out, pushing to timeout queue", signature.UUID)
		if err := r.pushToTimeoutAmqpQueue(signature); err == nil {
			_ = amqpJob.Ack(false)
		}
		_ = amqpJob.Nack(false, true)
		return nil
	}

	// Step 2: Validate task handler
	registeredTaskFunc, err := r.GetRegisteredTask(signature.Name)
	if err != nil || registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("Task is not registered or invalid")
		_ = amqpJob.Nack(multiple, requeue)
		return appError.ErrTaskMustBeFunc
	}

	// Step 3: Type assertion and execution
	taskFunc, ok := registeredTaskFunc.(func(*schema.Signature) error)
	if !ok {
		logger.ApplicationLogger.Error("Invalid task function type")
		_ = amqpJob.Nack(multiple, requeue)
		return appError.ErrTaskMustBeFunc
	}

	if err := taskFunc(signature); err != nil {
		// Retry path
		if retryErr := r.retryTask(signature); retryErr != nil {
			logger.ApplicationLogger.Error("Retry failed, requeuing")
			requeue = true
			_ = amqpJob.Nack(multiple, requeue)
			return retryErr
		}
		_ = amqpJob.Ack(false)
		return nil
	}
	// Success
	return amqpJob.Ack(false)
}

func (r *DefaultTaskRegistrar) checkTaskTimeout(signature *schema.Signature) bool {
	if signature.TaskTimeOut == 0 || signature.CreatedAt == nil {
		return false
	}

	elapsed := time.Since(*signature.CreatedAt).Seconds()
	return elapsed >= float64(signature.TaskTimeOut)
}

func (r *DefaultTaskRegistrar) pushToTimeoutAmqpQueue(signature *schema.Signature) error {
	signature.RoutingKey = r.configProvider.GetConfig().AMQP.TimeoutQueue
	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) retryTask(signature *schema.Signature) error {
	if signature.RetryCount < 1 {
		signature.RoutingKey = r.configProvider.GetConfig().AMQP.FailedQueue
	} else if signature.RetriesDone > (signature.RetryCount - 1) {
		signature.RoutingKey = r.configProvider.GetConfig().AMQP.FailedQueue
	} else {
		retryInterval := r.getRetryInterval(signature.RetriesDone + 1)
		if retryInterval > 0 {
			signature.RetriesDone = signature.RetriesDone + 1
			signature.RoutingKey = r.configProvider.GetConfig().AMQP.DelayedQueue
		}
		eta := time.Now().UTC().Add(retryInterval)
		signature.ETA = &eta
	}
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

	if signature.CreatedAt == nil {
		now := time.Now().UTC()
		signature.CreatedAt = &now
	}

	if err := r.broker.Publish(ctx, signature); err != nil {
		return fmt.Errorf("Publish message error: %s", err)
	}
	return nil
}
