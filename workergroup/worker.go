package workergroup

import (
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/task"
)

type WorkerGroup struct {
	Namespace      string
	Concurrency    uint
	WorkersChannel chan struct{}
	TaskRegistrar  task.TaskRegistrarInterface
	// Add other parameters as needed
}

func (w *WorkerGroup) Processor(signature *task.Signature) error {
	registeredTaskFunc, err := w.TaskRegistrar.GetRegisteredTask(signature.Name)
	if err != nil {
		logger.ApplicationLogger.Error("invalid task function")
		return errors.ErrTaskMustBeFunc
	} else if registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("task is not registered")
		return errors.ErrTaskNotRegistered
	}

	if fn, ok := registeredTaskFunc.(func(*task.Signature) error); ok {
		if err := fn(signature); err != nil {
			logger.ApplicationLogger.Error("task failed to execute")
			//TODO: retry
		}
	}
	return nil
}

func NewWorkerGroup(concurrency uint, registrarInterface task.TaskRegistrarInterface, namespace string) *WorkerGroup {
	workers := &WorkerGroup{
		Concurrency:   concurrency,
		TaskRegistrar: registrarInterface,
		Namespace:     namespace,
	}
	workers.WorkersChannel = make(chan struct{}, concurrency)
	for i := uint(0); i < concurrency; i++ {
		workers.WorkersChannel <- struct{}{}
	}
	return workers
}
