package memory

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/task"

	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/logger"
	"github.com/surendratiwari3/paota/internal/schema"
	appError "github.com/surendratiwari3/paota/internal/schema/errors"
	"github.com/surendratiwari3/paota/internal/utils"
	"github.com/surendratiwari3/paota/internal/validation"
	"sync"
	"time"
)

type DefaultTaskRegistrar struct {
	registeredTasks      *sync.Map
	registeredTasksCount uint
	broker               broker.Broker
}

func NewDefaultTaskRegistrar(brk broker.Broker) task.TaskRegistrarInterface {
	return &DefaultTaskRegistrar{
		registeredTasks: new(sync.Map),
		broker:          brk,
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
		return appError.ErrTaskMustBeFunc
	} else if registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("task is not registered")
		return appError.ErrTaskNotRegistered
	} else if fn, ok := registeredTaskFunc.(func(*schema.Signature) error); ok {
		if err := fn(signature); err == nil {
			return amqpJob.Ack(false)
		} else if err := r.retry(signature, err); err != nil {
			return amqpJob.Nack(false, true)
		} else {
			return amqpJob.Nack(false, false)
		}
	}

	return nil
}

func (r *DefaultTaskRegistrar) retry(signature *schema.Signature, err error) error {
	if signature.RetryCount < 1 {
		return err
	}
	signature.RetriesDone = signature.RetriesDone + 1
	if signature.RetriesDone > signature.RetryCount {
		return err
	}
	retryInterval := r.getRetryInterval(signature.RetriesDone)
	eta := time.Now().UTC().Add(retryInterval)
	signature.ETA = &eta

	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) getTaskTTL(task *schema.Signature) int64 {
	var delayMs int64
	if task.ETA != nil {
		now := time.Now().UTC()
		if task.ETA.After(now) {
			delayMs = int64(task.ETA.Sub(now) / time.Millisecond)
		}
	}
	if delayMs > 0 {
		task.RoutingKey = config.GetConfigProvider().GetConfig().AMQP.DelayedQueue
	}
	return delayMs
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
	if err := r.broker.Publish(ctx, signature); err != nil {
		return fmt.Errorf("Publish message error: %s", err)
	}
	return nil
}