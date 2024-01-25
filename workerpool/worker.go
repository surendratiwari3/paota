package workerpool

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/task"
	"reflect"
)

type worker struct {
	workerID    string
	poolID      string
	namespace   string
	contextType reflect.Type

	stopChan         chan struct{}
	doneStoppingChan chan struct{}

	drainChan        chan struct{}
	doneDrainingChan chan struct{}

	jobs chan task.Job
}

func newWorker(namespace string, poolID string, contextType reflect.Type) *worker {
	workerID := uuid.New().String()

	wrk := &worker{
		workerID:    workerID,
		poolID:      poolID,
		namespace:   namespace,
		contextType: contextType,

		stopChan:         make(chan struct{}),
		doneStoppingChan: make(chan struct{}),

		drainChan:        make(chan struct{}),
		doneDrainingChan: make(chan struct{}),
	}

	return wrk
}

func (w *worker) start() {
	go func() {
		select {
		case <-w.stopChan:
			w.doneStoppingChan <- struct{}{}
			return
		case job := <-w.jobs:
			fmt.Println(job)
			//process consuming
		}
	}()
}

func (w *worker) stop() {
	w.stopChan <- struct{}{}
	<-w.doneStoppingChan
}
