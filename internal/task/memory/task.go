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
	if err := r.broker.Publish(ctx, signature); err != nil {
		return fmt.Errorf("Publish message error: %s", err)
	}
	return nil
}
