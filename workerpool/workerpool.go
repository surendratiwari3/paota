package workerpool

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/broker"
	amqpBroker "github.com/surendratiwari3/paota/broker/amqp"
	"github.com/surendratiwari3/paota/config"
	appErrors "github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"
	"os"
	"os/signal"
	"reflect"

	"sync"
)

// WorkerPool stores all configuration for tasks workers
type WorkerPool struct {
	backend              store.Backend
	broker               broker.Broker
	config               *config.Config
	registeredTasks      *sync.Map
	registeredTasksCount uint
	started              bool
	workerPoolID         string
	concurrency          uint
	nameSpace            string
	contextType          reflect.Type
	taskChannel          chan task.Job
	ackChannel           chan uint64 // Channel to receive delivery tags for acknowledgments
}

type WorkerPoolOptions struct {
}

// NewWorkerPool creates WorkerPool instance
func NewWorkerPool(ctx interface{}, concurrency uint, nameSpace string) (Pool, error) {
	if err := validateContextType(ctx); err != nil {
		return nil, err
	}

	cnf := config.GetConfigProvider().GetConfig()
	if cnf == nil {
		return nil, errors.New("config is not provided")
	}

	workerPoolId := uuid.New().String()

	workerPool := &WorkerPool{
		concurrency:     concurrency,
		config:          cnf,
		contextType:     reflect.TypeOf(ctx),
		nameSpace:       nameSpace,
		registeredTasks: new(sync.Map),
		started:         false,
		workerPoolID:    workerPoolId,
	}

	err := workerPool.createBroker()
	if err != nil {
		logger.ApplicationLogger.Error("broker creation failed", err)
		return nil, err
	}

	// Backend is optional so we ignore the error
	err = workerPool.createStore()
	if err != nil {
		logger.ApplicationLogger.Error("store creation failed", err)
		return nil, err
	}

	return workerPool, nil
}

// NewWorkerPoolWithOptions : TODO future with options
func NewWorkerPoolWithOptions(ctx interface{}, concurrency uint, namespace string, workerPoolOpts WorkerPoolOptions) (Pool, error) {
	return NewWorkerPool(ctx, concurrency, namespace)
}

// GetBroker returns broker
func (wp *WorkerPool) GetBroker() broker.Broker {
	return wp.broker
}

// SetBroker sets broker
func (wp *WorkerPool) SetBroker(broker broker.Broker) {
	wp.broker = broker
}

// GetBackend returns backend
func (wp *WorkerPool) GetBackend() store.Backend {
	return wp.backend
}

// SetBackend sets backend
func (wp *WorkerPool) SetBackend(backend store.Backend) {
	wp.backend = backend
}

// RegisterTasks registers all tasks at once
func (wp *WorkerPool) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	for _, taskFunc := range namedTaskFuncs {
		if err := task.ValidateTask(taskFunc); err != nil {
			return err
		}
	}

	for k, v := range namedTaskFuncs {
		wp.registeredTasksCount = wp.registeredTasksCount + 1
		wp.registeredTasks.Store(k, v)
	}
	return nil
}

// IsTaskRegistered returns true if the task name is registered with this broker
func (wp *WorkerPool) IsTaskRegistered(name string) bool {
	_, ok := wp.registeredTasks.Load(name)
	return ok
}

// GetRegisteredTask returns registered task by name
func (wp *WorkerPool) GetRegisteredTask(name string) (interface{}, error) {
	taskFunc, ok := wp.registeredTasks.Load(name)
	if !ok {
		return nil, fmt.Errorf("Task not registered error: %s", name)
	}
	return taskFunc, nil
}

// SendTaskWithContext will inject the trace context in the signature headers before publishing it
func (wp *WorkerPool) SendTaskWithContext(ctx context.Context, signature *task.Signature) (*task.State, error) {
	// Auto generate a UUID if not set already
	if signature.UUID == "" {
		taskID := uuid.New().String()
		signature.UUID = fmt.Sprintf("task_%v", taskID)
	}

	if err := wp.broker.Publish(ctx, signature); err != nil {
		return nil, fmt.Errorf("Publish message error: %s", err)
	}

	// Set initial task state to PENDING
	/*if w.backend != nil {
		if err := w.backend.InsertTask(*signature); err != nil {
			// TODO: error handling as enqueue is already done, if this is happening after retry also
			// TODO: should we have different queue for backend also to handle such cases
			return nil, fmt.Errorf("Insert state error: %s", err)
		}
	}*/

	return task.NewPendingTaskState(signature), nil
}

// validateContextType will panic if context is invalid
func validateContextType(ctx interface{}) error {
	ctxType := reflect.TypeOf(ctx)
	if ctxType.Kind() != reflect.Struct {
		return errors.New("work: Context needs to be a struct type")
	}
	return nil
}

func (wp *WorkerPool) Start() error {
	logger.ApplicationLogger.Info("worker pool called")
	if wp.started {
		return nil
	}

	wp.started = true
	logger.ApplicationLogger.Info("worker pool called")

	go func() {
		for {
			workers := make(chan struct{}, wp.concurrency)
			wp.initializeWorkers(workers, wp.concurrency)
			err := wp.broker.StartConsumer(wp.nameSpace, workers, wp.registeredTasks)
			if err != nil {
				logger.ApplicationLogger.Error("consumer failed to start", err)
				return
			}
		}
	}()

	logger.ApplicationLogger.Info("worker pool started")
	// Wait for a signal to quit:
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	<-signalChan

	wp.Stop()
	//here now worker pool called this but as we know amqp consumer will be one but it will prefetch and now how to distribute
	return nil
}

// Stop stops the workers and associated processes.
func (wp *WorkerPool) Stop() {
	if !wp.started {
		return
	}

	if wp.registeredTasksCount == 0 {
		return
	}
	wp.started = false
	wp.broker.StopConsumer()
}

func (wp *WorkerPool) initializeWorkers(workers chan struct{}, concurrency uint) {
	for i := uint(0); i < concurrency; i++ {
		workers <- struct{}{}
	}
}

// CreateBroker creates a new object of broker.Broker
func (wp *WorkerPool) createBroker() error {
	brokerType := config.GetConfigProvider().GetConfig().Broker
	switch brokerType {
	case "amqp":
		newAmqpBroker, err := amqpBroker.NewAMQPBroker()
		if err != nil {
			return err
		}
		wp.broker = newAmqpBroker
	default:
		logger.ApplicationLogger.Error("unsupported broker")
		return appErrors.ErrUnsupportedBroker
	}
	return nil
}

// CreateStore creates a new object of store.Interface
func (wp *WorkerPool) createStore() error {
	storeBackend := config.GetConfigProvider().GetConfig().Store
	switch storeBackend {
	case "":
		return nil
	default:
		return appErrors.ErrUnsupportedStore
	}
}
