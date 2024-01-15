package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/backend"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"

	"sync"
)

// Server is the main Machinery object and stores all configuration
// All the tasks workers process are registered against the server
type Server struct {
	config          *config.Config
	registeredTasks *sync.Map
	broker          broker.Broker
	backend         backend.Backend
}

// NewServer creates Server instance
func NewServer(cnf *config.Config) (*Server, error) {
	broker, err := BrokerFactory(cnf)
	if err != nil {
		return nil, err
	}

	// Backend is optional so we ignore the error
	backend, err := BackendFactory(cnf)
	if err != nil {
		return nil, err
	}

	server := &Server{
		config:          cnf,
		registeredTasks: new(sync.Map),
		broker:          broker,
		backend:         backend,
	}

	return server, nil
}

// NewWorker creates Worker instance
func (server *Server) NewWorker(consumerTag string, concurrency int) *Worker {
	return &Worker{
		ConsumerTag: consumerTag,
		Concurrency: concurrency,
	}
}

// GetBroker returns broker
func (server *Server) GetBroker() broker.Broker {

	return server.broker
}

// SetBroker sets broker
func (server *Server) SetBroker(broker broker.Broker) {
	server.broker = broker
}

// GetBackend returns backend
func (server *Server) GetBackend() backend.Backend {
	return server.backend
}

// SetBackend sets backend
func (server *Server) SetBackend(backend backend.Backend) {
	server.backend = backend
}

// GetConfig returns connection object
func (server *Server) GetConfig() *config.Config {
	return server.config
}

// SetConfig sets config
func (server *Server) SetConfig(cnf *config.Config) {
	server.config = cnf
}

// RegisterTasks registers all tasks at once
func (server *Server) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	for _, taskFunc := range namedTaskFuncs {
		if err := task.ValidateTask(taskFunc); err != nil {
			return err
		}
	}

	for k, v := range namedTaskFuncs {
		server.registeredTasks.Store(k, v)
	}
	return nil
}

// RegisterTask registers a single task
func (server *Server) RegisterTask(name string, taskFunc interface{}) error {
	if err := task.ValidateTask(taskFunc); err != nil {
		return err
	}
	server.registeredTasks.Store(name, taskFunc)
	return nil
}

// IsTaskRegistered returns true if the task name is registered with this broker
func (server *Server) IsTaskRegistered(name string) bool {
	_, ok := server.registeredTasks.Load(name)
	return ok
}

// GetRegisteredTask returns registered task by name
func (server *Server) GetRegisteredTask(name string) (interface{}, error) {
	taskFunc, ok := server.registeredTasks.Load(name)
	if !ok {
		return nil, fmt.Errorf("Task not registered error: %s", name)
	}
	return taskFunc, nil
}

// SendTaskWithContext will inject the trace context in the signature headers before publishing it
func (server *Server) SendTaskWithContext(ctx context.Context, signature *task.Signature) (*task.State, error) {

	// Make sure result backend is defined
	if server.backend == nil {
		return nil, errors.New("Result backend required")
	}

	// Auto generate a UUID if not set already
	if signature.UUID == "" {
		taskID := uuid.New().String()
		signature.UUID = fmt.Sprintf("task_%v", taskID)
	}

	// Set initial task state to PENDING
	if err := server.backend.InsertTask(*signature); err != nil {
		return nil, fmt.Errorf("Insert state error: %s", err)
	}

	if err := server.broker.Enqueue(ctx, signature); err != nil {
		return nil, fmt.Errorf("Publish message error: %s", err)
	}

	return task.NewPendingTaskState(signature), nil
}

// SendTask publishes a task to the default queue
func (server *Server) SendTask(signature *task.Signature) (*task.State, error) {
	return server.SendTaskWithContext(context.Background(), signature)
}
