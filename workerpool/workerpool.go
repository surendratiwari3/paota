package workerpool

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"
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
	workers              []*worker
	taskChannel          chan task.Job
	ackChannel           chan uint64 // Channel to receive delivery tags for acknowledgments
}

type WorkerPoolOptions struct {
}

// NewWorkerPool creates WorkerPool instance
func NewWorkerPool(ctx interface{}, concurrency uint, nameSpace string) (*WorkerPool, error) {
	if err := validateContextType(ctx); err != nil {
		return nil, err
	}

	cnf := config.GetConfigProvider().GetConfig()
	if cnf == nil {
		return nil, errors.New("config is not provided")
	}

	taskChannel := make(chan task.Job)

	taskBroker, err := CreateBroker(taskChannel)
	if err != nil {
		return nil, err
	}

	// Backend is optional so we ignore the error
	backend, err := CreateStore()
	if err != nil {
		return nil, err
	}

	workerPoolId := uuid.New().String()

	workerPool := &WorkerPool{
		config:          cnf,
		registeredTasks: new(sync.Map),
		broker:          taskBroker,
		backend:         backend,
		workerPoolID:    workerPoolId,
		started:         false,
		concurrency:     concurrency,
		nameSpace:       nameSpace,
		contextType:     reflect.TypeOf(ctx),
		taskChannel:     taskChannel,
	}

	for i := uint(0); i < workerPool.concurrency; i++ {
		w := newWorker(workerPool.nameSpace, workerPool.workerPoolID, workerPool.contextType)
		workerPool.workers = append(workerPool.workers, w)
	}

	return workerPool, nil
}

// NewWorkerPoolWithOptions : TODO future with options
func NewWorkerPoolWithOptions(ctx interface{}, concurrency uint, namespace string, workerPoolOpts WorkerPoolOptions) (*WorkerPool, error) {
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

// GetConfig returns connection object
func (wp *WorkerPool) GetConfig() *config.Config {
	return wp.config
}

// SetConfig sets config
func (wp *WorkerPool) SetConfig(cnf *config.Config) {
	wp.config = cnf
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

// SendTask publishes a task to the default queue
func (wp *WorkerPool) SendTask(signature *task.Signature) (*task.State, error) {
	return wp.SendTaskWithContext(context.Background(), signature)
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
	for _, w := range wp.workers {
		go w.start()
	}
	wp.started = true
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

	wg := sync.WaitGroup{}
	for _, w := range wp.workers {
		wg.Add(1)
		go func(w *worker) {
			w.stop()
			wg.Done()
		}(w)
	}
	wg.Wait()
}

func (wp *WorkerPool) AddJob(job task.Job) {
	wp.taskChannel <- job
}

func (wp *WorkerPool) Acknowledge(tag uint64) {
	wp.ackChannel <- tag
}