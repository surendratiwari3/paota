//go:build integration

// Package integration exercises paota against a real RabbitMQ broker.
//
// These tests are behind the `integration` build tag so `go test ./...` stays
// hermetic. Run them with:
//
//	docker run -d --name paota-rmq -p 5672:5672 rabbitmq:3.13-management
//	go test -tags=integration -v ./integration/...
//
// Environment:
//
//	PAOTA_TEST_AMQP_URL           broker URL (default amqp://guest:guest@localhost:5672/)
//	PAOTA_TEST_RABBITMQ_CONTAINER docker container name; required by the
//	                              broker-restart tests, which are skipped
//	                              without it.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
)

const (
	defaultAMQPURL = "amqp://guest:guest@localhost:5672/"
	taskName       = "integrationTask"
)

func amqpURL() string {
	if url := os.Getenv("PAOTA_TEST_AMQP_URL"); url != "" {
		return url
	}
	return defaultAMQPURL
}

func containerName() string {
	return os.Getenv("PAOTA_TEST_RABBITMQ_CONTAINER")
}

// requireBroker skips the test when no broker is reachable, so a developer
// without Docker gets a clear skip instead of a confusing failure.
func requireBroker(t *testing.T) {
	t.Helper()
	conn, err := amqp.Dial(amqpURL())
	if err != nil {
		t.Skipf("no RabbitMQ at %s: %v", amqpURL(), err)
	}
	_ = conn.Close()
}

// testTopology returns a set of resource names unique to one test, so tests
// never consume each other's messages.
type testTopology struct {
	Exchange     string
	TaskQueue    string
	DelayedQueue string
	FailedQueue  string
	TimeoutQueue string
	BindingKey   string
}

func newTopology(t *testing.T) testTopology {
	t.Helper()
	id := fmt.Sprintf("%s_%d", sanitize(t.Name()), time.Now().UnixNano())
	return testTopology{
		Exchange:     "paota_it_exchange_" + id,
		TaskQueue:    "paota_it_tasks_" + id,
		DelayedQueue: "paota_it_delayed_" + id,
		FailedQueue:  "paota_it_failed_" + id,
		TimeoutQueue: "paota_it_timeout_" + id,
		BindingKey:   "paota_it_key_" + id,
	}
}

func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func (topo testTopology) config(concurrencyPrefetch int) config.Config {
	return config.Config{
		Broker:                   "amqp",
		TaskQueueName:            topo.TaskQueue,
		MaxTaskExecutionDuration: 30,
		AMQP: &config.AMQPConfig{
			Url:                   amqpURL(),
			Exchange:              topo.Exchange,
			ExchangeType:          "direct",
			BindingKey:            topo.BindingKey,
			DelayedQueue:          topo.DelayedQueue,
			FailedQueue:           topo.FailedQueue,
			TimeoutQueue:          topo.TimeoutQueue,
			PrefetchCount:         concurrencyPrefetch,
			ConnectionPoolSize:    2,
			HeartBeatInterval:     5,
			ConnectionTimeout:     5,
			PublishConfirmTimeout: 10,
		},
	}
}

// cleanup deletes everything the test declared so repeated runs start clean.
func (topo testTopology) cleanup(t *testing.T) {
	t.Helper()
	conn, err := amqp.Dial(amqpURL())
	if err != nil {
		t.Logf("cleanup: cannot connect: %v", err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Logf("cleanup: cannot open channel: %v", err)
		return
	}
	defer ch.Close()

	for _, q := range []string{topo.TaskQueue, topo.DelayedQueue, topo.FailedQueue, topo.TimeoutQueue} {
		if _, err := ch.QueueDelete(q, false, false, false); err != nil {
			t.Logf("cleanup: delete queue %s: %v", q, err)
			// A failed delete closes the channel, so open a fresh one.
			if ch, err = conn.Channel(); err != nil {
				return
			}
		}
	}
	if err := ch.ExchangeDelete(topo.Exchange, false, false); err != nil {
		t.Logf("cleanup: delete exchange %s: %v", topo.Exchange, err)
	}
}

// payload is the body of an integration task.
type payload struct {
	Index int    `json:"index"`
	Batch string `json:"batch"`
}

func newSignature(index int, batch string) *schema.Signature {
	body, _ := json.Marshal(payload{Index: index, Batch: batch})
	return &schema.Signature{
		Name:       taskName,
		RawArgs:    body,
		RetryCount: 3,
	}
}

// recorder tracks which messages were processed, and how many ran at once.
type recorder struct {
	mu              sync.Mutex
	seen            map[int]int
	inFlight        atomic.Int64
	maxInFlight     atomic.Int64
	processDuration time.Duration
}

func newRecorder(processDuration time.Duration) *recorder {
	return &recorder{seen: make(map[int]int), processDuration: processDuration}
}

func (r *recorder) task(sig *schema.Signature) error {
	current := r.inFlight.Add(1)
	for {
		max := r.maxInFlight.Load()
		if current <= max || r.maxInFlight.CompareAndSwap(max, current) {
			break
		}
	}
	defer r.inFlight.Add(-1)

	if r.processDuration > 0 {
		time.Sleep(r.processDuration)
	}

	var p payload
	if err := json.Unmarshal(sig.RawArgs, &p); err != nil {
		return err
	}

	r.mu.Lock()
	r.seen[p.Index]++
	r.mu.Unlock()
	return nil
}

func (r *recorder) distinct() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

// missing returns the indices below total that were never processed.
func (r *recorder) missing(total int) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []int
	for i := 0; i < total; i++ {
		if r.seen[i] == 0 {
			out = append(out, i)
		}
	}
	return out
}

func (r *recorder) duplicates() map[int]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	dups := make(map[int]int)
	for idx, count := range r.seen {
		if count > 1 {
			dups[idx] = count
		}
	}
	return dups
}

// startPool builds a worker pool, registers the recorder task and runs it.
// The returned stop function shuts the pool down and waits for Start to return.
func startPool(t *testing.T, topo testTopology, concurrency uint, rec *recorder) (workerpool.Pool, func()) {
	t.Helper()

	pool, err := workerpool.NewWorkerPoolWithConfig(struct{}{}, concurrency, "integration", topo.config(int(concurrency)))
	require.NoError(t, err)
	require.NoError(t, pool.RegisterTasks(map[string]interface{}{
		taskName: rec.task,
	}))

	done := make(chan error, 1)
	go func() { done <- pool.Start() }()

	var stopOnce sync.Once
	return pool, func() {
		stopOnce.Do(func() {
			pool.Stop()
			select {
			case <-done:
			case <-time.After(60 * time.Second):
				t.Error("worker pool did not shut down")
			}
		})
	}
}

// publishAll publishes count messages through a publisher-only pool.
func publishAll(t *testing.T, pool workerpool.Pool, count int, batch string) {
	t.Helper()
	for i := 0; i < count; i++ {
		_, err := pool.SendTaskWithContext(context.Background(), newSignature(i, batch))
		require.NoErrorf(t, err, "publishing message %d", i)
	}
}

// restartBroker restarts the RabbitMQ container and waits for it to accept
// connections again.
func restartBroker(t *testing.T) {
	t.Helper()
	name := containerName()
	if name == "" {
		t.Skip("set PAOTA_TEST_RABBITMQ_CONTAINER to run broker restart tests")
	}

	out, err := exec.Command("docker", "restart", name).CombinedOutput()
	require.NoErrorf(t, err, "docker restart %s: %s", name, out)

	waitForBroker(t, 90*time.Second)
}

// stopBroker stops the container without starting it again.
func stopBroker(t *testing.T) {
	t.Helper()
	name := containerName()
	if name == "" {
		t.Skip("set PAOTA_TEST_RABBITMQ_CONTAINER to run broker outage tests")
	}
	out, err := exec.Command("docker", "stop", name).CombinedOutput()
	require.NoErrorf(t, err, "docker stop %s: %s", name, out)
}

func startBroker(t *testing.T) {
	t.Helper()
	name := containerName()
	out, err := exec.Command("docker", "start", name).CombinedOutput()
	require.NoErrorf(t, err, "docker start %s: %s", name, out)
	waitForBroker(t, 90*time.Second)
}

func waitForBroker(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := amqp.Dial(amqpURL())
		if err == nil {
			ch, chErr := conn.Channel()
			if chErr == nil {
				_ = ch.Close()
				_ = conn.Close()
				// Give the broker a moment to finish recovering its queues.
				time.Sleep(500 * time.Millisecond)
				return
			}
			_ = conn.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("RabbitMQ did not become available within %s", timeout)
}

// queueDepth reports how many messages are sitting in a queue.
func queueDepth(t *testing.T, queue string) int {
	t.Helper()
	conn, err := amqp.Dial(amqpURL())
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// Passive declare so the check never creates the queue by accident.
	q, err := ch.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		return 0
	}
	return q.Messages
}

// eventually polls until condition holds, failing with msg on timeout.
func eventually(t *testing.T, timeout time.Duration, condition func() bool, msg string, args ...interface{}) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf(msg, args...)
}
