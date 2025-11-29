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
	amqpJob, ok := job.(amqp.Delivery)
	if !ok {
		return errors.ErrUnsupported
	}

	if len(amqpJob.Body) == 0 {
		logger.ApplicationLogger.Error("empty message, return")
		_ = amqpJob.Nack(false, false)
		return appError.ErrEmptyMessage
	}

	var signature *schema.Signature
	var err error

	// Detect new-style message: metadata in headers
	if _, ok := amqpJob.Headers["task_name"]; ok {
		// Rebuild Signature from headers + RawArgs
		signature, err = r.SignatureFromAmqpHeadersAndBody(amqpJob.Headers, amqpJob.Body)
		if err != nil {
			logger.ApplicationLogger.Error("failed to build signature from headers and body", err)
			_ = amqpJob.Nack(false, false)
			return err
		}
	} else {
		// Fallback: full Signature in body
		signature, err = schema.BytesToSignature(amqpJob.Body)
		if err != nil {
			logger.ApplicationLogger.Error("decode error in message, return")
			_ = amqpJob.Nack(false, false)
			return err
		}
	}

	return r.amqpProcessSignature(amqpJob, signature)
}

func (r *DefaultTaskRegistrar) amqpProcessSignature(amqpJob amqp.Delivery, signature *schema.Signature) error {
	multiple := false
	requeue := false

	// Step 1: Timeout check
	if r.checkTaskTimeout(signature) {
		logger.ApplicationLogger.Warnf("Task %s timed out, pushing to timeout queue", signature.UUID)
		if err := r.pushToTimeoutAmqpQueue(signature); err == nil {
			_ = amqpJob.Nack(false, false)
			return nil
		}
		logger.ApplicationLogger.Error("Task is not timeout and push to timeout queue failed")
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
	tq := r.configProvider.GetConfig().AMQP.TimeoutQueue
	if tq == "" {
		// nothing to do if timeout queue not configured
		return nil
	}
	signature.TaskTimeOut = 0 //this will help to ensure task should not get timeout while processing it from timeout queue
	signature.RoutingKey = tq
	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) retryTask(signature *schema.Signature) error {
	cfg := r.configProvider.GetConfig().AMQP

	// Case 1: No retries configured or exhausted
	if signature.RetryCount < 1 || signature.RetriesDone > (signature.RetryCount-1) {
		if cfg.FailedQueue == "" {
			// No failed queue configured, just skip
			// You can either log a warning or return an error here
			return nil
		}
		signature.RoutingKey = cfg.FailedQueue
		return r.SendTask(signature)
	}

	// Case 2: Retry allowed
	retryInterval := r.getRetryInterval(signature.RetriesDone + 1)
	if retryInterval > 0 {
		signature.RetriesDone++
		signature.RoutingKey = cfg.DelayedQueue
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

func (r *DefaultTaskRegistrar) SignatureFromAmqpHeadersAndBody(headers amqp.Table, body []byte) (*schema.Signature, error) {
	s := &schema.Signature{
		Name:                        utils.GetString(headers["task_name"]),
		UUID:                        utils.GetString(headers["uuid"]),
		RawArgs:                     body,
		RoutingKey:                  utils.GetString(headers["routing_key"]),
		Priority:                    utils.GetUInt8(headers["priority"]),
		RetryCount:                  utils.GetInt(headers["retry_count"]),
		RetryTimeout:                utils.GetInt(headers["retry_timeout"]),
		WaitTime:                    utils.GetInt(headers["wait_time"]),
		RetriesDone:                 utils.GetInt(headers["retries_done"]),
		TaskTimeOut:                 utils.GetInt(headers["timeout"]),
		IgnoreWhenTaskNotRegistered: utils.GetBool(headers["ignore_if_unregistered"]),
	}

	if createdAtUnix, ok := utils.GetInt64Ok(headers["created_at"]); ok {
		t := time.Unix(createdAtUnix, 0).UTC() // parse as UTC
		s.CreatedAt = &t
	}
	if etaUnix, ok := utils.GetInt64Ok(headers["eta"]); ok {
		t := time.Unix(etaUnix, 0).UTC() // parse as UTC
		s.ETA = &t
	}

	return s, nil
}
