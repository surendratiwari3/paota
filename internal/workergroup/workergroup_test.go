package workergroup

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/internal/task"
)

// fakeRegistrar is a hand written stub so these tests do not depend on
// generated mocks (mocks are gitignored and produced by mockery in CI).
type fakeRegistrar struct {
	task.TaskRegistrarInterface

	processed atomic.Int64
	inFlight  atomic.Int64
	maxInFlig atomic.Int64
	err       error
	panicWith atomic.Value
	block     chan struct{}
	seen      chan interface{}
}

func newFakeRegistrar() *fakeRegistrar {
	return &fakeRegistrar{seen: make(chan interface{}, 1024)}
}

func (f *fakeRegistrar) Processor(job interface{}) error {
	current := f.inFlight.Add(1)
	for {
		max := f.maxInFlig.Load()
		if current <= max || f.maxInFlig.CompareAndSwap(max, current) {
			break
		}
	}
	defer f.inFlight.Add(-1)

	if f.block != nil {
		<-f.block
	}
	if p := f.panicWith.Load(); p != nil {
		panic(p)
	}

	f.processed.Add(1)
	select {
	case f.seen <- job:
	default:
	}
	return f.err
}

func TestNewWorkerGroup(t *testing.T) {
	wg := NewWorkerGroup(4, newFakeRegistrar(), "namespace")

	require.NotNil(t, wg)
	assert.Equal(t, "namespace", wg.GetWorkerGroupName())

	group, ok := wg.(*workerGroup)
	require.True(t, ok)
	assert.Len(t, group.Workers, 4)
	assert.Equal(t, uint(4), group.Concurrency)
}

// TestAssignJobWithZeroConcurrency covers the integer divide by zero that used
// to crash the whole process the first time a delivery arrived on a group
// created with concurrency 0.
func TestAssignJobWithZeroConcurrency(t *testing.T) {
	wg := NewWorkerGroup(0, newFakeRegistrar(), "namespace")

	group, ok := wg.(*workerGroup)
	require.True(t, ok)
	assert.Len(t, group.Workers, 1, "a group must always have at least one worker")

	wg.Start()
	defer wg.Stop()

	assert.NotPanics(t, func() { wg.AssignJob("job") })
}

func TestAssignJobProcessesJob(t *testing.T) {
	registrar := newFakeRegistrar()
	wg := NewWorkerGroup(2, registrar, "namespace")

	wg.Start()
	defer wg.Stop()

	wg.AssignJob("job-1")

	select {
	case job := <-registrar.seen:
		assert.Equal(t, "job-1", job)
	case <-time.After(2 * time.Second):
		t.Fatal("job was never processed")
	}
}

func TestAssignJobRoundRobinsAcrossWorkers(t *testing.T) {
	registrar := newFakeRegistrar()
	wg := NewWorkerGroup(3, registrar, "namespace")
	group := wg.(*workerGroup)

	wg.Start()
	defer wg.Stop()

	for i := 0; i < 6; i++ {
		wg.AssignJob(i)
	}

	require.Eventually(t, func() bool {
		return registrar.processed.Load() == 6
	}, 5*time.Second, 10*time.Millisecond)

	// Index wraps around the worker slice rather than growing without bound.
	group.mu.Lock()
	defer group.mu.Unlock()
	assert.Less(t, group.lastAssignedWorkerIndex, len(group.Workers))
	assert.GreaterOrEqual(t, group.lastAssignedWorkerIndex, 0)
}

// TestConcurrentProcessing proves the group actually runs jobs in parallel:
// with N blocked workers, N jobs must be in flight simultaneously.
func TestConcurrentProcessing(t *testing.T) {
	const concurrency = 5

	registrar := newFakeRegistrar()
	registrar.block = make(chan struct{})

	wg := NewWorkerGroup(concurrency, registrar, "namespace")
	wg.Start()

	var assigned sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		assigned.Add(1)
		go func(i int) {
			defer assigned.Done()
			wg.AssignJob(i)
		}(i)
	}

	require.Eventually(t, func() bool {
		return registrar.inFlight.Load() == concurrency
	}, 5*time.Second, 10*time.Millisecond, "expected %d jobs in flight", concurrency)

	close(registrar.block)
	assigned.Wait()

	require.Eventually(t, func() bool {
		return registrar.processed.Load() == concurrency
	}, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, int64(concurrency), registrar.maxInFlig.Load())

	wg.Stop()
}

// TestStopIsIdempotent covers the "close of closed channel" panic that a second
// Stop used to trigger — for example when both a signal handler and an explicit
// Stop call raced during shutdown.
func TestStopIsIdempotent(t *testing.T) {
	wg := NewWorkerGroup(2, newFakeRegistrar(), "namespace")
	wg.Start()

	assert.NotPanics(t, func() {
		wg.Stop()
		wg.Stop()
		wg.Stop()
	})
}

func TestStopWithoutStart(t *testing.T) {
	wg := NewWorkerGroup(2, newFakeRegistrar(), "namespace")

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Stop()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked on a group that was never started")
	}
}

// TestAssignJobAfterStop covers the "send on closed channel" panic: a delivery
// that arrived while the pool was shutting down used to crash the process.
func TestAssignJobAfterStop(t *testing.T) {
	registrar := newFakeRegistrar()
	wg := NewWorkerGroup(2, registrar, "namespace")

	wg.Start()
	wg.Stop()

	assert.NotPanics(t, func() {
		wg.AssignJob("late-job")
	})
	assert.Equal(t, int64(0), registrar.processed.Load())
}

// TestAssignJobUnblocksOnStop makes sure a producer waiting for a busy worker
// is released by Stop instead of deadlocking the shutdown.
func TestAssignJobUnblocksOnStop(t *testing.T) {
	registrar := newFakeRegistrar()
	registrar.block = make(chan struct{})

	wg := NewWorkerGroup(1, registrar, "namespace")
	wg.Start()

	// Occupy the only worker.
	wg.AssignJob("first")
	require.Eventually(t, func() bool {
		return registrar.inFlight.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)

	blocked := make(chan struct{})
	go func() {
		defer close(blocked)
		wg.AssignJob("second") // no free worker: this parks
	}()

	// The producer must still be waiting.
	select {
	case <-blocked:
		t.Fatal("AssignJob returned even though every worker is busy")
	case <-time.After(100 * time.Millisecond):
	}

	go wg.Stop()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("AssignJob did not return after Stop")
	}

	close(registrar.block)
}

// TestPanickingTaskDoesNotKillWorker: a panic inside a task used to take down
// the worker goroutine, silently reducing the group's concurrency.
func TestPanickingTaskDoesNotKillWorker(t *testing.T) {
	registrar := newFakeRegistrar()
	registrar.panicWith.Store("boom")

	wg := NewWorkerGroup(1, registrar, "namespace")
	wg.Start()
	defer wg.Stop()

	assert.NotPanics(t, func() { wg.AssignJob("panics") })

	// The same worker must still accept the next job.
	require.Eventually(t, func() bool {
		return registrar.inFlight.Load() == 0
	}, 5*time.Second, 10*time.Millisecond)
	registrar.panicWith = atomic.Value{}

	wg.AssignJob("survives")

	select {
	case job := <-registrar.seen:
		assert.Equal(t, "survives", job)
	case <-time.After(5 * time.Second):
		t.Fatal("worker died after a panicking task")
	}
}

func TestProcessorErrorIsNotFatal(t *testing.T) {
	registrar := newFakeRegistrar()
	registrar.err = errors.New("task failed")

	wg := NewWorkerGroup(1, registrar, "namespace")
	wg.Start()
	defer wg.Stop()

	wg.AssignJob("job-1")
	wg.AssignJob("job-2")

	require.Eventually(t, func() bool {
		return registrar.processed.Load() == 2
	}, 5*time.Second, 10*time.Millisecond)
}

func TestStartIsIdempotent(t *testing.T) {
	registrar := newFakeRegistrar()
	wg := NewWorkerGroup(2, registrar, "namespace")

	wg.Start()
	wg.Start() // must not spawn a second set of workers
	defer wg.Stop()

	wg.AssignJob("job")

	require.Eventually(t, func() bool {
		return registrar.processed.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestStopWaitsForInFlightJob(t *testing.T) {
	registrar := newFakeRegistrar()
	registrar.block = make(chan struct{})

	wg := NewWorkerGroup(1, registrar, "namespace")
	wg.Start()

	wg.AssignJob("slow")
	require.Eventually(t, func() bool {
		return registrar.inFlight.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		wg.Stop()
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned while a job was still running")
	case <-time.After(150 * time.Millisecond):
	}

	close(registrar.block)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after the in-flight job finished")
	}
}
