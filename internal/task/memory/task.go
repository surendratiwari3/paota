package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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
)

// defaultMaxTaskExecutionDuration is used when the configuration does not
// specify one. Without this fallback a zero value produced an already-expired
// context and every task was reported as timed out.
const defaultMaxTaskExecutionDuration = 10 * time.Second

// maxRetryInterval caps the Fibonacci backoff between retries.
// TODO max cap move to config
const maxRetryInterval = 10 * time.Second

type DefaultTaskRegistrar struct {
	registeredTasks *sync.Map
	// registeredTasksCount is read from other goroutines during shutdown, so it
	// is accessed atomically rather than as a plain uint.
	registeredTasksCount atomic.Uint64
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
		// Only count a name the first time it is seen; re-registering a task
		// used to inflate the count forever.
		if _, loaded := r.registeredTasks.Swap(k, v); !loaded {
			r.registeredTasksCount.Add(1)
		}
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
	return uint(r.registeredTasksCount.Load())
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

// getConfig returns the active configuration, never nil.
func (r *DefaultTaskRegistrar) getConfig() *config.Config {
	if r.configProvider == nil {
		return &config.Config{}
	}
	cfg := r.configProvider.GetConfig()
	if cfg == nil {
		return &config.Config{}
	}
	return cfg
}

// getAMQPConfig returns the AMQP configuration, never nil, so the retry and
// timeout paths cannot panic on a partially configured worker.
func (r *DefaultTaskRegistrar) getAMQPConfig() *config.AMQPConfig {
	if amqpCfg := r.getConfig().AMQP; amqpCfg != nil {
		return amqpCfg
	}
	return &config.AMQPConfig{}
}

func (r *DefaultTaskRegistrar) maxTaskExecutionDuration() time.Duration {
	if secs := r.getConfig().MaxTaskExecutionDuration; secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return defaultMaxTaskExecutionDuration
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

	if signature == nil || signature.Name == "" {
		logger.ApplicationLogger.Error("message carries no task name, dropping")
		_ = amqpJob.Nack(false, false)
		return appError.ErrTaskNotRegistered
	}

	return r.amqpProcessSignature(amqpJob, signature)
}

func (r *DefaultTaskRegistrar) amqpProcessSignature(amqpJob amqp.Delivery, signature *schema.Signature) (err error) {

	multiple := false
	requeue := false

	maxExec := r.maxTaskExecutionDuration()

	ctx, cancel := context.WithTimeout(context.Background(), maxExec)
	defer cancel()

	done := make(chan error, 1)

	// Safety net: a panic raised by this function itself (not by the task,
	// which runs in its own goroutine below) must not leave the message
	// unacknowledged and the worker dead.
	defer func() {
		if rec := recover(); rec != nil {
			logger.ApplicationLogger.Errorf(
				"panic while processing task uuid=%s name=%s err=%v",
				signature.UUID,
				signature.Name,
				rec,
			)

			// panic = retry if possible
			if retryErr := r.retryTask(signature); retryErr != nil {
				_ = amqpJob.Nack(false, true)
				err = fmt.Errorf("panic while processing task: %v", rec)
				return
			}
			_ = amqpJob.Ack(false)
			err = fmt.Errorf("panic while processing task: %v", rec)
		}
	}()

	// Step 1: START-time timeout (queue wait)
	if r.checkTaskTimeout(signature) {
		logger.ApplicationLogger.Warnf(
			"Task %s exceeded start timeout, pushing to timeout queue",
			signature.UUID,
		)

		if err := r.pushToTimeoutAmqpQueue(signature); err == nil {
			_ = amqpJob.Ack(false)
			return nil
		}

		// Could not park the task: requeue it so it is not lost.
		_ = amqpJob.Nack(false, true)
		return nil
	}

	// Step 2: Get task
	registeredTaskFunc, err := r.GetRegisteredTask(signature.Name)
	if err != nil || registeredTaskFunc == nil {
		return r.handleUnregisteredTask(amqpJob, signature)
	}

	taskFunc, ok := registeredTaskFunc.(func(*schema.Signature) error)
	if !ok {
		logger.ApplicationLogger.Error("Invalid task function type")
		_ = amqpJob.Nack(multiple, requeue)
		return appError.ErrTaskMustBeFunc
	}

	// Step 3: Execute task in goroutine (IMPORTANT)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("task panic: %v", rec)
			}
		}()
		done <- taskFunc(signature)
	}()

	// Step 4: Wait for completion OR timeout
	select {
	case err := <-done:
		if err != nil {
			logger.ApplicationLogger.Warnf("Task %s failed: %v", signature.UUID, err)
			// Retry on error
			if retryErr := r.retryTask(signature); retryErr != nil {
				_ = amqpJob.Nack(false, true)
				return retryErr
			}
			_ = amqpJob.Ack(false)
			return nil
		}

		// Success
		return amqpJob.Ack(false)

	case <-ctx.Done():
		// Execution timeout reached
		logger.ApplicationLogger.Warnf(
			"Task %s exceeded max execution time (%s), retrying",
			signature.UUID,
			maxExec,
		)

		if retryErr := r.retryTask(signature); retryErr != nil {
			_ = amqpJob.Nack(false, true)
			return retryErr
		}

		// IMPORTANT: ACK original message
		_ = amqpJob.Ack(false)
		return nil
	}
}

// handleUnregisteredTask decides what to do with a message whose task name this
// worker does not know about. Dropping it outright (the previous behaviour)
// silently lost work whenever a message reached a worker that had not
// registered that task yet — for example during a rolling deploy.
func (r *DefaultTaskRegistrar) handleUnregisteredTask(amqpJob amqp.Delivery, signature *schema.Signature) error {
	if signature.IgnoreWhenTaskNotRegistered {
		logger.ApplicationLogger.Warnf("Task %s is not registered, ignoring as requested", signature.Name)
		_ = amqpJob.Ack(false)
		return appError.ErrTaskNotRegistered
	}

	if failedQueue := r.getAMQPConfig().FailedQueue; failedQueue != "" {
		logger.ApplicationLogger.Errorf("Task %s is not registered, parking it in %s", signature.Name, failedQueue)
		parked := *signature
		parked.RoutingKey = failedQueue
		if err := r.SendTask(&parked); err == nil {
			_ = amqpJob.Ack(false)
			return appError.ErrTaskNotRegistered
		}
		logger.ApplicationLogger.Error("failed to park unregistered task, requeueing")
		_ = amqpJob.Nack(false, true)
		return appError.ErrTaskNotRegistered
	}

	logger.ApplicationLogger.Errorf("Task %s is not registered and no failed queue is configured, dropping", signature.Name)
	_ = amqpJob.Nack(false, false)
	return appError.ErrTaskNotRegistered
}

func (r *DefaultTaskRegistrar) checkTaskTimeout(signature *schema.Signature) bool {
	if signature.TaskTimeOut == 0 || signature.CreatedAt == nil {
		return false
	}

	elapsed := time.Since(*signature.CreatedAt).Seconds()
	return elapsed >= float64(signature.TaskTimeOut)
}

func (r *DefaultTaskRegistrar) pushToTimeoutAmqpQueue(signature *schema.Signature) error {
	tq := r.getAMQPConfig().TimeoutQueue
	if tq == "" {
		// nothing to do if timeout queue not configured
		return nil
	}
	signature.TaskTimeOut = 0 //this will help to ensure task should not get timeout while processing it from timeout queue
	signature.RoutingKey = tq
	// A parked task must not inherit a stale ETA, which would have the broker
	// hold it in the delay queue instead of the timeout queue.
	signature.ETA = nil
	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) retryTask(signature *schema.Signature) error {
	cfg := r.getAMQPConfig()

	// Case 1: No retries configured or exhausted
	if signature.RetryCount < 1 || signature.RetriesDone > (signature.RetryCount-1) {
		logger.ApplicationLogger.Warnf(
			"Task %s failed after all retry exhausted, pushing to failed queue",
			signature.UUID,
		)
		if cfg.FailedQueue == "" {
			// No failed queue configured, just skip
			// You can either log a warning or return an error here
			return nil
		}
		signature.RoutingKey = cfg.FailedQueue
		signature.ETA = nil
		return r.SendTask(signature)
	}

	// Case 2: Retry allowed
	retryInterval := r.getRetryInterval(signature.RetriesDone + 1)
	signature.RetriesDone++
	if retryInterval > 0 && cfg.DelayedQueue != "" {
		signature.RoutingKey = cfg.DelayedQueue
		eta := time.Now().UTC().Add(retryInterval)
		signature.ETA = &eta
	} else {
		// Without a delay queue the task goes straight back to its own queue;
		// keeping a stale ETA here would have it published with an expiration.
		signature.ETA = nil
	}
	return r.SendTask(signature)
}

func (r *DefaultTaskRegistrar) getRetryInterval(retryCount int) time.Duration {
	// Base Fibonacci delay, in seconds.
	//
	// The cap is applied to the number of seconds rather than to the resulting
	// Duration: Fibonacci(60) seconds does not fit in an int64 of nanoseconds,
	// so converting first overflowed into a large negative delay that skipped
	// the cap entirely and published the retry with no backoff at all.
	seconds := utils.Fibonacci(retryCount)
	maxSeconds := int(maxRetryInterval / time.Second)
	if seconds < 0 || seconds > maxSeconds {
		return maxRetryInterval
	}

	return time.Duration(seconds) * time.Second
}

func (r *DefaultTaskRegistrar) SendTask(signature *schema.Signature) error {
	return r.SendTaskWithContext(context.Background(), signature)
}

func (r *DefaultTaskRegistrar) SendTaskWithContext(ctx context.Context, signature *schema.Signature) error {
	if signature == nil {
		return appError.ErrEmptyMessage
	}
	if r.broker == nil {
		return appError.ErrUnsupportedBroker
	}

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
		return fmt.Errorf("publish message error: %w", err)
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
