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
	started                 bool
	stopped                 bool
	// quit is closed by Stop and unblocks both idle workers and producers that
	// are waiting to hand a job over.
	quit      chan struct{}
	workersWG sync.WaitGroup
	// Add other parameters as needed
}

// NewWorkerGroup - create a workergroup
func NewWorkerGroup(concurrency uint, registrarInterface task.TaskRegistrarInterface, namespace string) WorkerGroupInterface {
	// A group with no workers can never run anything and used to make
	// AssignJob panic with an integer divide by zero, so fall back to a
	// single worker.
	if concurrency == 0 {
		concurrency = 1
	}

	wrkGrp := &workerGroup{
		Concurrency:             concurrency,
		TaskRegistrar:           registrarInterface,
		Namespace:               namespace,
		lastAssignedWorkerIndex: -1,
		quit:                    make(chan struct{}),
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

// Start spins up one goroutine per worker. Calling it more than once is a no-op
// so a restarted pool does not end up with duplicated workers.
func (wg *workerGroup) Start() {
	wg.mu.Lock()
	if wg.started || wg.stopped {
		wg.mu.Unlock()
		return
	}
	wg.started = true
	workers := wg.Workers
	wg.mu.Unlock()

	for _, wrk := range workers {
		wg.workersWG.Add(1)
		go wg.work(wrk)
	}
}

// Stop signals all workers to exit and waits for the in-flight jobs to finish.
//
// Stop is idempotent: the previous implementation closed every job channel
// directly, so a second call panicked with "close of closed channel" and any
// producer blocked in AssignJob panicked with "send on closed channel".
func (wg *workerGroup) Stop() {
	wg.mu.Lock()
	if wg.stopped {
		wg.mu.Unlock()
		return
	}
	wg.stopped = true
	close(wg.quit)
	started := wg.started
	wg.mu.Unlock()

	if started {
		wg.workersWG.Wait()
	}
}

func (wg *workerGroup) work(wrk *worker) {
	defer wg.workersWG.Done()
	for {
		select {
		case job := <-wrk.JobChannel:
			wg.process(job)
		case <-wg.quit:
			return
		}
	}
}

// process runs a single job and keeps a panicking task from taking the whole
// worker goroutine (and therefore the process) down with it.
func (wg *workerGroup) process(job interface{}) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.ApplicationLogger.Error("panic while executing task - ", rec)
		}
	}()

	if err := wg.TaskRegistrar.Processor(job); err != nil {
		logger.ApplicationLogger.Error("error while executing task - ", err)
	}
}

// AssignJob assigns a job to a worker in a round-robin fashion.
// It returns without dispatching once the group has been stopped.
func (wg *workerGroup) AssignJob(job interface{}) {
	wg.mu.Lock()
	if wg.stopped || len(wg.Workers) == 0 {
		wg.mu.Unlock()
		logger.ApplicationLogger.Warning("worker group is not accepting jobs, dropping job")
		return
	}

	wg.lastAssignedWorkerIndex = (wg.lastAssignedWorkerIndex + 1) % len(wg.Workers)
	selectedWorker := wg.Workers[wg.lastAssignedWorkerIndex]
	wg.mu.Unlock()

	// Hand the job over outside the lock so a busy worker cannot block Stop,
	// and bail out if the group is stopped while we wait.
	select {
	case selectedWorker.JobChannel <- job:
	case <-wg.quit:
		logger.ApplicationLogger.Warning("worker group stopped while assigning job, dropping job")
	}
}

func (wg *workerGroup) GetWorkerGroupName() string {
	return wg.Namespace
}
