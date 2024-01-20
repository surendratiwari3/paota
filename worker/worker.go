package worker

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"

	"sync"
)

// Worker stores all configuration for tasks workers
type Worker struct {
	config          *config.Config
	registeredTasks *sync.Map
	broker          broker.Broker
	backend         store.Backend
}

// NewWorker creates Worker instance
func NewWorker(cnf *config.Config) (*Worker, error) {
	broker, err := CreateBroker(cnf)
	if err != nil {
		return nil, err
	}

	// Backend is optional so we ignore the error
	backend, err := CreateStore(cnf)
	if err != nil {
		return nil, err
	}

	worker := &Worker{
		config:          cnf,
		registeredTasks: new(sync.Map),
		broker:          broker,
		backend:         backend,
	}

	return worker, nil
}

// GetBroker returns broker
func (w *Worker) GetBroker() broker.Broker {

	return w.broker
}

// SetBroker sets broker
func (w *Worker) SetBroker(broker broker.Broker) {
	w.broker = broker
}

// GetBackend returns backend
func (w *Worker) GetBackend() store.Backend {
	return w.backend
}

// SetBackend sets backend
func (w *Worker) SetBackend(backend store.Backend) {
	w.backend = backend
}

// GetConfig returns connection object
func (w *Worker) GetConfig() *config.Config {
	return w.config
}

// SetConfig sets config
func (w *Worker) SetConfig(cnf *config.Config) {
	w.config = cnf
}

// RegisterTasks registers all tasks at once
func (w *Worker) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	for _, taskFunc := range namedTaskFuncs {
		if err := task.ValidateTask(taskFunc); err != nil {
			return err
		}
	}

	for k, v := range namedTaskFuncs {
		w.registeredTasks.Store(k, v)
	}
	return nil
}

// IsTaskRegistered returns true if the task name is registered with this broker
func (w *Worker) IsTaskRegistered(name string) bool {
	_, ok := w.registeredTasks.Load(name)
	return ok
}

// GetRegisteredTask returns registered task by name
func (w *Worker) GetRegisteredTask(name string) (interface{}, error) {
	taskFunc, ok := w.registeredTasks.Load(name)
	if !ok {
		return nil, fmt.Errorf("Task not registered error: %s", name)
	}
	return taskFunc, nil
}

// SendTaskWithContext will inject the trace context in the signature headers before publishing it
func (w *Worker) SendTaskWithContext(ctx context.Context, signature *task.Signature) (*task.State, error) {
	// Auto generate a UUID if not set already
	if signature.UUID == "" {
		taskID := uuid.New().String()
		signature.UUID = fmt.Sprintf("task_%v", taskID)
	}

	if err := w.broker.Publish(ctx, signature); err != nil {
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
func (w *Worker) SendTask(signature *task.Signature) (*task.State, error) {
	return w.SendTaskWithContext(context.Background(), signature)
}
