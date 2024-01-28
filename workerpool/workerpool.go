package workerpool

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/workergroup"
	"os"
	"os/signal"
	"reflect"

	"sync"
)

// WorkerPool stores all configuration for tasks workers
type WorkerPool struct {
	backend       store.Backend
	broker        broker.Broker
	factory       IFactory
	config        *config.Config
	taskRegistrar task.TaskRegistrarInterface
	started       bool
	workerPoolID  string
	concurrency   uint
	nameSpace     string
	contextType   reflect.Type
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
		concurrency:  concurrency,
		config:       cnf,
		contextType:  reflect.TypeOf(ctx),
		nameSpace:    nameSpace,
		started:      false,
		workerPoolID: workerPoolId,
		factory:      globalFactory,
	}

	if workerPool.factory == nil {
		workerPool.factory = &Factory{}
	}

	factoryBroker, err := workerPool.factory.CreateBroker()
	if err != nil {
		logger.ApplicationLogger.Error("broker creation failed", err)
		return nil, err
	}
	workerPool.broker = factoryBroker

	// Backend is optional so we ignore the error
	err = workerPool.factory.CreateStore()
	if err != nil {
		logger.ApplicationLogger.Error("store creation failed", err)
		return nil, err
	}

	taskRegistrar := workerPool.factory.CreateTaskRegistrar()
	if taskRegistrar == nil {
		logger.ApplicationLogger.Error("task registrar creation failed")
		return nil, errors.New("failed to start the worker pool")
	}
	workerPool.taskRegistrar = taskRegistrar

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
	if wp.started {
		return nil
	}

	wp.started = true
	logger.ApplicationLogger.Info("worker pool called")

	var signalWG sync.WaitGroup
	go func() {
		for {
			workers := workergroup.NewWorkerGroup(wp.concurrency, wp.taskRegistrar, wp.nameSpace)
			err := wp.broker.StartConsumer(workers)
			if err != nil {
				logger.ApplicationLogger.Error("consumer failed to start", err)
				return
			}
			signalWG.Wait()
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

func (wp *WorkerPool) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	return wp.taskRegistrar.RegisterTasks(namedTaskFuncs)
}

func (wp *WorkerPool) IsTaskRegistered(name string) bool {
	return wp.taskRegistrar.IsTaskRegistered(name)
}

// Stop stops the workers and associated processes.
func (wp *WorkerPool) Stop() {
	if !wp.started {
		return
	}

	if wp.taskRegistrar.GetRegisteredTaskCount() == 0 {
		return
	}
	wp.started = false
	wp.broker.StopConsumer()
}
