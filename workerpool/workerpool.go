package workerpool

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/factory"
	"github.com/surendratiwari3/paota/internal/store"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/schema/errors"

	"os"
	"os/signal"
	"reflect"

	"github.com/surendratiwari3/paota/internal/workergroup"

	"sync"
)

// WorkerPool stores all configuration for tasks workers
type WorkerPool struct {
	backend        store.Backend
	broker         broker.Broker
	failOverBroker broker.Broker
	factory        factory.IFactory
	config         *config.Config
	taskRegistrar  task.TaskRegistrarInterface
	started        bool
	workerPoolID   string
	concurrency    uint
	nameSpace      string
	contextType    reflect.Type
	workerGroup    workergroup.WorkerGroupInterface
}

// globalFactory defined just for unit test cases
var (
	globalFactory factory.IFactory
)

type WorkerPoolOptions struct {
}

// NewWorkerPool creates WorkerPool instance
func NewWorkerPool(ctx interface{}, concurrency uint, nameSpace string) (Pool, error) {
	if err := validateContextType(ctx); err != nil {
		return nil, err
	}

	cnf := config.GetConfigProvider().GetConfig()
	if cnf == nil {
		return nil, errors.ErrNilConfig
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
		workerPool.factory = &factory.Factory{}
	}

	factoryBroker, err := workerPool.factory.CreateBroker("master")
	if err != nil {
		logger.ApplicationLogger.Error("master broker creation failed", err)
		return nil, err
	}
	workerPool.broker = factoryBroker

	if cnf.AmqpFailover != nil {
		if cnf.FailoverQueueName == "" {
			return nil, errors.ErrInvalidConfig
		}
		failOverBroker, err := workerPool.factory.CreateBroker("failover")
		if err != nil {
			logger.ApplicationLogger.Error("failover  broker creation failed", err)
			return nil, err
		}
		workerPool.failOverBroker = failOverBroker
	}

	// Backend is optional so we ignore the error
	err = workerPool.factory.CreateStore()
	if err != nil {
		logger.ApplicationLogger.Error("store creation failed", err)
		return nil, err
	}

	taskRegistrar := workerPool.factory.CreateTaskRegistrar(workerPool.broker, workerPool.failOverBroker)
	if taskRegistrar == nil {
		logger.ApplicationLogger.Error("task registrar creation failed")
		return nil, errors.ErrFailedToStartWorkerPool
	}
	workerPool.taskRegistrar = taskRegistrar

	workerPool.workerGroup = workergroup.NewWorkerGroup(concurrency, taskRegistrar, nameSpace)

	return workerPool, nil
}

func NewWorkerPoolWithConfig(ctx interface{}, concurrency uint, nameSpace string, conf config.Config) (Pool, error) {
	err := config.GetConfigProvider().SetApplicationConfig(conf)
	if err != nil {
		logger.ApplicationLogger.Error("config error", err)
		return nil, err
	}
	return NewWorkerPool(ctx, concurrency, nameSpace)
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
func (wp *WorkerPool) SendTaskWithContext(ctx context.Context, signature *schema.Signature) (*schema.State, error) {
	if err := wp.taskRegistrar.SendTaskWithContext(ctx, signature); err != nil {
		return nil, fmt.Errorf("publish message error: %s", err)
	}
	return schema.NewPendingTaskState(signature), nil
}

// validateContextType will panic if context is invalid
func validateContextType(ctx interface{}) error {
	ctxType := reflect.TypeOf(ctx)
	if ctxType.Kind() != reflect.Struct {
		return errors.ErrInvalidWorkerContext
	}
	return nil
}

func (wp *WorkerPool) Start() error {
	if wp.started {
		return nil
	}

	wp.started = true
	logger.ApplicationLogger.Info("worker pool called")

	wp.workerGroup.Start()

	var signalWG sync.WaitGroup
	go func() {
		for {
			err := wp.broker.StartConsumer(context.Background(), wp.workerGroup)
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
	wp.workerGroup.Stop()
	wp.broker.StopConsumer()

}
