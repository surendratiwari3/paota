package workergroup

import (
	"errors"
	appError "github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/utils"
	"time"
)

type WorkerGroupInterface interface {
	Processor(signature *schema.Signature) error
	GetWorkerGroupName() string
	GetWorker()
	AddWorker()
}

type workerGroup struct {
	Namespace      string
	Concurrency    uint
	WorkersChannel chan struct{}
	TaskRegistrar  task.TaskRegistrarInterface
	// Add other parameters as needed
}

func (w *workerGroup) Processor(signature *schema.Signature) error {
	registeredTaskFunc, err := w.TaskRegistrar.GetRegisteredTask(signature.Name)
	if err != nil {
		logger.ApplicationLogger.Error("invalid task function")
		return appError.ErrTaskMustBeFunc
	} else if registeredTaskFunc == nil {
		logger.ApplicationLogger.Error("task is not registered")
		return appError.ErrTaskNotRegistered
	}

	if fn, ok := registeredTaskFunc.(func(*schema.Signature) error); ok {
		if err := fn(signature); err != nil {
			logger.ApplicationLogger.Error("task failed to execute")
			return w.parseRetry(signature, err)
		}
	}
	return nil
}

// getRetry
func (w *workerGroup) parseRetry(signature *schema.Signature, err error) error {
	if signature.RetryCount < 1 {
		return err
	}
	signature.RetriesDone = signature.RetriesDone + 1
	if signature.RetriesDone > signature.RetryCount {
		return err
	}
	retryInterval := w.getRetryInterval(signature.RetriesDone)
	eta := time.Now().UTC().Add(retryInterval)
	signature.ETA = &eta
	retryError := appError.RetryError{
		RetryCount: signature.RetryCount,
		RetryAfter: retryInterval,
		Err:        errors.New("try again later"),
	}
	return &retryError
}

// getRetryInterval
func (w *workerGroup) getRetryInterval(retryCount int) time.Duration {
	return time.Duration(utils.Fibonacci(retryCount)) * time.Second
}

func (w *workerGroup) GetWorkerGroupName() string {
	return w.Namespace
}

func (w *workerGroup) GetWorker() {
	<-w.WorkersChannel
}

func (w *workerGroup) AddWorker() {
	w.WorkersChannel <- struct{}{}
}

// NewWorkerGroup - create a workergroup
func NewWorkerGroup(concurrency uint, registrarInterface task.TaskRegistrarInterface, namespace string) WorkerGroupInterface {
	wrkGrp := &workerGroup{
		Concurrency:   concurrency,
		TaskRegistrar: registrarInterface,
		Namespace:     namespace,
	}
	wrkGrp.WorkersChannel = make(chan struct{}, concurrency)
	for i := uint(0); i < concurrency; i++ {
		wrkGrp.AddWorker()
	}
	return wrkGrp
}
