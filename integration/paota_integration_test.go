//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
)

// TestPublishAndConsume is the baseline: everything that is published is
// processed, exactly once, with nothing left behind in the queue.
func TestPublishAndConsume(t *testing.T) {
	requireBroker(t)
	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	const messages = 200

	rec := newRecorder(0)
	pool, stop := startPool(t, topo, 4, rec)
	defer stop()

	publishAll(t, pool, messages, "baseline")

	eventually(t, 60*time.Second, func() bool {
		return rec.distinct() == messages
	}, "only %d/%d messages were processed; missing %v", rec.distinct(), messages, rec.missing(messages))

	assert.Empty(t, rec.missing(messages), "messages were lost")
	assert.Empty(t, rec.duplicates(), "messages were processed more than once")

	stop()
	assert.Equal(t, 0, queueDepth(t, topo.TaskQueue), "queue should be drained")
}

// TestConcurrentProcessing checks that a pool with N workers really processes N
// messages at a time rather than serialising them.
func TestConcurrentProcessing(t *testing.T) {
	requireBroker(t)
	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	const (
		concurrency = 8
		messages    = 40
		taskTime    = 200 * time.Millisecond
	)

	rec := newRecorder(taskTime)
	pool, stop := startPool(t, topo, concurrency, rec)
	defer stop()

	publishAll(t, pool, messages, "concurrent")

	start := time.Now()
	eventually(t, 90*time.Second, func() bool {
		return rec.distinct() == messages
	}, "only %d/%d messages were processed", rec.distinct(), messages)
	elapsed := time.Since(start)

	serialTime := time.Duration(messages) * taskTime
	assert.Less(t, elapsed, serialTime,
		"processing took %s, which is no better than running the %d tasks serially (%s)", elapsed, messages, serialTime)

	// Several tasks must have overlapped. Being strict about hitting exactly
	// `concurrency` would make this flaky on a loaded CI box, so require a
	// clear majority of the workers to have been busy at once.
	assert.GreaterOrEqual(t, rec.maxInFlight.Load(), int64(concurrency/2),
		"expected concurrent execution, peak in-flight was %d", rec.maxInFlight.Load())
	assert.Empty(t, rec.missing(messages))
}

// TestBrokerRestartNoMessageLoss is the core durability test: RabbitMQ is
// restarted while the consumer is working through a backlog, and every message
// must still be processed.
func TestBrokerRestartNoMessageLoss(t *testing.T) {
	requireBroker(t)
	if containerName() == "" {
		t.Skip("set PAOTA_TEST_RABBITMQ_CONTAINER to run broker restart tests")
	}

	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	const messages = 300

	// Slow enough that the restart lands in the middle of the backlog.
	rec := newRecorder(20 * time.Millisecond)
	pool, stop := startPool(t, topo, 4, rec)
	defer stop()

	publishAll(t, pool, messages, "restart")

	// Wait until the consumer is clearly under way, then pull the broker out.
	eventually(t, 60*time.Second, func() bool {
		return rec.distinct() > 10
	}, "consumer never started processing")

	processedBeforeRestart := rec.distinct()
	t.Logf("restarting broker after %d/%d messages", processedBeforeRestart, messages)
	restartBroker(t)

	// The pool must reconnect on its own and finish the backlog.
	eventually(t, 180*time.Second, func() bool {
		return rec.distinct() == messages
	}, "after the restart only %d/%d messages were processed; missing %v",
		rec.distinct(), messages, rec.missing(messages))

	assert.Empty(t, rec.missing(messages), "messages were lost across the broker restart")
	assert.Greater(t, rec.distinct(), processedBeforeRestart,
		"no progress was made after the restart, so the consumer never reconnected")
}

// TestConsumerReconnectsAfterOutage covers a broker that is down for a while:
// the pool has to keep retrying and pick up work once RabbitMQ is back.
func TestConsumerReconnectsAfterOutage(t *testing.T) {
	requireBroker(t)
	if containerName() == "" {
		t.Skip("set PAOTA_TEST_RABBITMQ_CONTAINER to run broker outage tests")
	}

	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	const messages = 50

	rec := newRecorder(0)
	pool, stop := startPool(t, topo, 4, rec)
	defer stop()

	// Prime the topology and confirm the consumer works before the outage.
	publishAll(t, pool, 1, "warmup")
	eventually(t, 30*time.Second, func() bool {
		return rec.distinct() >= 1
	}, "consumer never processed the warm-up message")

	stopBroker(t)
	// Give the pool time to notice and enter its reconnect loop.
	time.Sleep(3 * time.Second)
	startBroker(t)

	// Publishing works again once the connection pool has healed.
	var published int
	eventually(t, 90*time.Second, func() bool {
		_, err := pool.SendTaskWithContext(context.Background(), newSignature(published+1, "after-outage"))
		if err != nil {
			t.Logf("publish still failing: %v", err)
			return false
		}
		published++
		return true
	}, "publisher never recovered after the broker came back")

	for i := published + 1; i < messages; i++ {
		_, err := pool.SendTaskWithContext(context.Background(), newSignature(i, "after-outage"))
		require.NoErrorf(t, err, "publishing message %d after recovery", i)
	}

	eventually(t, 180*time.Second, func() bool {
		return rec.distinct() >= messages-1
	}, "consumer processed only %d/%d messages after the outage", rec.distinct(), messages-1)
}

// TestGracefulShutdownDrainsInFlightWork: messages already handed to a worker
// must finish, and anything still queued must stay in RabbitMQ rather than
// being acknowledged and dropped.
func TestGracefulShutdownDrainsInFlightWork(t *testing.T) {
	requireBroker(t)
	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	const messages = 100

	rec := newRecorder(50 * time.Millisecond)
	pool, stop := startPool(t, topo, 2, rec)

	publishAll(t, pool, messages, "shutdown")

	eventually(t, 60*time.Second, func() bool {
		return rec.distinct() > 5
	}, "consumer never started")

	processed := rec.distinct()
	stop()

	// Nothing may vanish: whatever was not processed is still on the queue.
	remaining := queueDepth(t, topo.TaskQueue)
	t.Logf("processed %d, still queued %d", processed, remaining)
	assert.GreaterOrEqual(t, rec.distinct()+remaining, messages,
		"processed (%d) + queued (%d) is less than the %d published messages",
		rec.distinct(), remaining, messages)
}

// TestFailedTaskIsRetriedThenParked walks a task through the retry ladder and
// checks it ends up on the failed queue instead of disappearing.
func TestFailedTaskIsRetriedThenParked(t *testing.T) {
	requireBroker(t)
	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	cfg := topo.config(4)
	pool, err := workerpool.NewWorkerPoolWithConfig(struct{}{}, 2, "integration", cfg)
	require.NoError(t, err)

	var attempts sync.Map
	var total int64
	var mu sync.Mutex

	require.NoError(t, pool.RegisterTasks(map[string]interface{}{
		taskName: func(sig *schema.Signature) error {
			mu.Lock()
			total++
			mu.Unlock()
			attempts.Store(sig.UUID, sig.RetriesDone)
			return fmt.Errorf("task always fails")
		},
	}))

	done := make(chan error, 1)
	go func() { done <- pool.Start() }()
	defer func() {
		pool.Stop()
		<-done
	}()

	sig := newSignature(0, "retry")
	sig.RetryCount = 2
	_, err = pool.SendTaskWithContext(context.Background(), sig)
	require.NoError(t, err)

	// Attempt 1 plus two retries; the Fibonacci backoff makes this take a few
	// seconds.
	eventually(t, 120*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return total >= 3
	}, "task was retried %d times, expected 3 attempts", total)

	// After the retries are exhausted the task is parked, not dropped.
	eventually(t, 60*time.Second, func() bool {
		return queueDepth(t, topo.FailedQueue) > 0
	}, "the exhausted task never reached the failed queue")
}

// TestUnregisteredTaskIsParkedNotDropped: a message for a task this worker does
// not know about used to be nacked without requeue and lost.
func TestUnregisteredTaskIsParkedNotDropped(t *testing.T) {
	requireBroker(t)
	topo := newTopology(t)
	t.Cleanup(func() { topo.cleanup(t) })

	rec := newRecorder(0)
	pool, stop := startPool(t, topo, 2, rec)
	defer stop()

	unknown := newSignature(0, "unknown")
	unknown.Name = "taskThisWorkerDoesNotKnow"
	_, err := pool.SendTaskWithContext(context.Background(), unknown)
	require.NoError(t, err)

	eventually(t, 60*time.Second, func() bool {
		return queueDepth(t, topo.FailedQueue) > 0
	}, "the unregistered task was dropped instead of being parked on the failed queue")

	assert.Equal(t, 0, rec.distinct(), "the unknown task must not have been executed")
}
