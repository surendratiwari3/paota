package workergroup

import (
	"errors"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/logger"
	"github.com/surendratiwari3/paota/internal/schema"
	appError "github.com/surendratiwari3/paota/internal/schema/errors"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/internal/utils"
	"time"
)

type WorkerGroupInterface interface {
	GetWorkerGroupName() string
	AssignJob(interface{})
	Stop()
	Start()
}

// Worker represents a worker that processes tasks.
type worker struct {
	ID         uint
	JobChannel chan interface{}
}

type workerGroup struct {
	Namespace     string
	Concurrency   uint
	Workers       []*worker
	TaskRegistrar task.TaskRegistrarInterface
	// Add other parameters as needed
}

// NewWorkerGroup - create a workergroup
func NewWorkerGroup(concurrency uint, registrarInterface task.TaskRegistrarInterface, namespace string) WorkerGroupInterface {
	wrkGrp := &workerGroup{
		Concurrency:   concurrency,
		TaskRegistrar: registrarInterface,
		Namespace:     namespace,
	}
	for i := uint(0); i < concurrency; i++ {
		worker := &worker{
			ID:         i,
			JobChannel: make(chan interface{}),
		}
		wrkGrp.Workers = append(wrkGrp.Workers, worker)
	}

	return wrkGrp
}

func (wg *workerGroup) Start() {
	for i := uint(0); i < wg.Concurrency; i++ {
		go wg.work(wg.Workers[i])
	}
}

// Stop all workers by closing their job channels.
func (wg *workerGroup) Stop() {
	for _, worker := range wg.Workers {
		close(worker.JobChannel)
	}
}

func (wg *workerGroup) work(wrk *worker) {
	for job := range wrk.JobChannel {
		err := wg.TaskRegistrar.Processor(job)
		if err != nil {
			logger.ApplicationLogger.Error("error while executing task")
		}
	}
}

// AssignJob assigns a job to a worker in a round-robin fashion.
func (wg *workerGroup) AssignJob(job interface{}) {
	// Use round-robin scheduling to assign jobs to workers
	for _, worker := range wg.Workers {
		worker.JobChannel <- job
	}
}

// getRetry
func (wg *workerGroup) parseRetry(signature *schema.Signature, err error) error {
	if signature.RetryCount < 1 {
		return err
	}
	signature.RetriesDone = signature.RetriesDone + 1
	if signature.RetriesDone > signature.RetryCount {
		return err
	}
	retryInterval := wg.getRetryInterval(signature.RetriesDone)
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
func (wg *workerGroup) getRetryInterval(retryCount int) time.Duration {
	return time.Duration(utils.Fibonacci(retryCount)) * time.Second
}

func (wg *workerGroup) GetWorkerGroupName() string {
	return wg.Namespace
}

func (wg *workerGroup) getTaskTTL(task *schema.Signature) int64 {
	var delayMs int64
	if task.ETA != nil {
		now := time.Now().UTC()
		if task.ETA.After(now) {
			delayMs = int64(task.ETA.Sub(now) / time.Millisecond)
		}
	}
	if delayMs > 0 {
		task.RoutingKey = config.GetConfigProvider().GetConfig().AMQP.DelayedQueue
	}
	return delayMs
}
