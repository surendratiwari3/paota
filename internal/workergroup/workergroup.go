package workergroup

import (
	"sync"

	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/logger"
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
	Namespace               string
	Concurrency             uint
	Workers                 []*worker
	TaskRegistrar           task.TaskRegistrarInterface
	lastAssignedWorkerIndex int
	mu                      sync.Mutex
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
	wg.mu.Lock()
	defer wg.mu.Unlock()

	wg.lastAssignedWorkerIndex = (wg.lastAssignedWorkerIndex + 1) % len(wg.Workers)

	selectedWorker := wg.Workers[wg.lastAssignedWorkerIndex]
	selectedWorker.JobChannel <- job
}

func (wg *workerGroup) GetWorkerGroupName() string {
	return wg.Namespace
}
