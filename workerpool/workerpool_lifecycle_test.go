package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/factory"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/schema"
)

// --- test doubles -----------------------------------------------------------

type fakeBroker struct {
	startCalls  atomic.Int64
	stopCalls   atomic.Int64
	startErr    error
	blockUntil  chan struct{}
	stopOrder   *orderRecorder
	consumerCtx context.Context
}

func (f *fakeBroker) StartConsumer(ctx context.Context, _ workergroup.WorkerGroupInterface) error {
	f.startCalls.Add(1)
	f.consumerCtx = ctx
	if f.blockUntil != nil {
		<-f.blockUntil
	}
	return f.startErr
}

func (f *fakeBroker) StopConsumer() {
	f.stopCalls.Add(1)
	if f.stopOrder != nil {
		f.stopOrder.record("broker.StopConsumer")
	}
	if f.blockUntil != nil {
		select {
		case <-f.blockUntil:
		default:
			close(f.blockUntil)
		}
	}
}

func (f *fakeBroker) Publish(context.Context, *schema.Signature) error { return nil }

type fakeWorkerGroup struct {
	started   atomic.Bool
	stopped   atomic.Bool
	stopOrder *orderRecorder
}

func (f *fakeWorkerGroup) GetWorkerGroupName() string { return "test" }
func (f *fakeWorkerGroup) AssignJob(interface{})      {}
func (f *fakeWorkerGroup) Start()                     { f.started.Store(true) }
func (f *fakeWorkerGroup) Stop() {
	f.stopped.Store(true)
	if f.stopOrder != nil {
		f.stopOrder.record("workerGroup.Stop")
	}
}

type orderRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (o *orderRecorder) record(step string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.steps = append(o.steps, step)
}

func (o *orderRecorder) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.steps...)
}

type fakeRegistrar struct {
	task.TaskRegistrarInterface
	sendErr error
}

func (f *fakeRegistrar) SendTaskWithContext(context.Context, *schema.Signature) error {
	return f.sendErr
}
func (f *fakeRegistrar) GetRegisteredTaskCount() uint { return 0 }

type fakeFactory struct {
	brk       broker.Broker
	registrar task.TaskRegistrarInterface
	brokerErr error
	storeErr  error
}

func (f *fakeFactory) CreateBroker(config.ConfigProvider) (broker.Broker, error) {
	if f.brokerErr != nil {
		return nil, f.brokerErr
	}
	return f.brk, nil
}
func (f *fakeFactory) CreateStore(config.ConfigProvider) error { return f.storeErr }
func (f *fakeFactory) CreateTaskRegistrar(broker.Broker, config.ConfigProvider) task.TaskRegistrarInterface {
	return f.registrar
}

type stubProvider struct{ cfg *config.Config }

func (s *stubProvider) ReadFromEnv() error                       { return nil }
func (s *stubProvider) SetApplicationConfig(config.Config) error { return nil }
func (s *stubProvider) ValidateConfig(config.Config) error       { return nil }
func (s *stubProvider) GetConfig() *config.Config                { return s.cfg }

func lifecycleConfig() *config.Config {
	return &config.Config{
		Broker:        "amqp",
		TaskQueueName: "paota_tasks",
		AMQP:          &config.AMQPConfig{Url: "amqp://localhost:5672", PrefetchCount: 10},
	}
}

// newTestPool builds a pool wired to fakes, bypassing the real factory.
func newTestPool(t *testing.T, brk broker.Broker, wg workergroup.WorkerGroupInterface, registrar task.TaskRegistrarInterface) *WorkerPool {
	t.Helper()
	if registrar == nil {
		registrar = &fakeRegistrar{}
	}
	return &WorkerPool{
		concurrency:    1,
		config:         lifecycleConfig(),
		nameSpace:      "test",
		workerPoolID:   "test-pool",
		configProvider: &stubProvider{cfg: lifecycleConfig()},
		broker:         brk,
		workerGroup:    wg,
		taskRegistrar:  registrar,
		stopChan:       make(chan struct{}),
		consumerDone:   make(chan struct{}),
	}
}

// --- construction -----------------------------------------------------------

func TestValidateContextType(t *testing.T) {
	assert.NoError(t, validateContextType(struct{}{}))
	assert.NoError(t, validateContextType(struct{ UserID string }{}))
	assert.Error(t, validateContextType("string"))
	assert.Error(t, validateContextType(42))
	// A nil context has no reflect.Type at all; this used to panic.
	assert.NotPanics(t, func() { assert.Error(t, validateContextType(nil)) })
}

func TestCreateWorkerPoolRequiresConfig(t *testing.T) {
	f := &fakeFactory{brk: &fakeBroker{}, registrar: &fakeRegistrar{}}
	globalFactory = f
	t.Cleanup(func() { globalFactory = nil })

	pool, err := createWorkerPool(struct{}{}, 1, "test", nil)
	assert.Nil(t, pool)
	assert.Error(t, err)

	pool, err = createWorkerPool(struct{}{}, 1, "test", &stubProvider{cfg: nil})
	assert.Nil(t, pool)
	assert.Error(t, err)
}

// TestCreateWorkerPoolClampsZeroConcurrency: a pool with no workers can never
// process anything, and used to crash on the first delivery.
func TestCreateWorkerPoolClampsZeroConcurrency(t *testing.T) {
	globalFactory = &fakeFactory{brk: &fakeBroker{}, registrar: &fakeRegistrar{}}
	t.Cleanup(func() { globalFactory = nil })

	pool, err := createWorkerPool(struct{}{}, 0, "test", &stubProvider{cfg: lifecycleConfig()})

	require.NoError(t, err)
	require.NotNil(t, pool)
	assert.Equal(t, uint(1), pool.(*WorkerPool).concurrency)
}

func TestCreateWorkerPoolPropagatesFactoryErrors(t *testing.T) {
	provider := &stubProvider{cfg: lifecycleConfig()}
	t.Cleanup(func() { globalFactory = nil })

	globalFactory = &fakeFactory{brokerErr: errors.New("broker down")}
	pool, err := createWorkerPool(struct{}{}, 1, "test", provider)
	assert.Nil(t, pool)
	assert.EqualError(t, err, "broker down")

	globalFactory = &fakeFactory{brk: &fakeBroker{}, storeErr: errors.New("store down")}
	pool, err = createWorkerPool(struct{}{}, 1, "test", provider)
	assert.Nil(t, pool)
	assert.EqualError(t, err, "store down")

	globalFactory = &fakeFactory{brk: &fakeBroker{}, registrar: nil}
	pool, err = createWorkerPool(struct{}{}, 1, "test", provider)
	assert.Nil(t, pool)
	assert.Error(t, err)
}

// --- lifecycle --------------------------------------------------------------

// TestStartStopDoesNotKillTheProcess is the regression test for os.Exit(0)
// being called from a library goroutine during an ordinary shutdown: if that
// came back, this test binary would simply vanish.
func TestStartStopDoesNotKillTheProcess(t *testing.T) {
	brk := &fakeBroker{startErr: errors.New("consumer exited")}
	wg := &fakeWorkerGroup{}
	pool := newTestPool(t, brk, wg, nil)

	started := make(chan error, 1)
	go func() { started <- pool.Start() }()

	require.Eventually(t, func() bool {
		return brk.startCalls.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	pool.Stop()

	select {
	case err := <-started:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}

	assert.True(t, wg.started.Load())
	assert.True(t, wg.stopped.Load())
	assert.GreaterOrEqual(t, brk.stopCalls.Load(), int64(1))
}

// TestShutdownOrder: the consumer has to be stopped before the workers are torn
// down, otherwise an in-flight delivery is handed to a closed worker channel.
func TestShutdownOrder(t *testing.T) {
	order := &orderRecorder{}
	brk := &fakeBroker{startErr: errors.New("stopped"), stopOrder: order}
	wg := &fakeWorkerGroup{stopOrder: order}
	pool := newTestPool(t, brk, wg, nil)

	go func() { _ = pool.Start() }()
	require.Eventually(t, func() bool {
		return brk.startCalls.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	pool.Stop()

	assert.Equal(t, []string{"broker.StopConsumer", "workerGroup.Stop"}, order.snapshot())
}

func TestStopIsIdempotentAndSafeBeforeStart(t *testing.T) {
	brk := &fakeBroker{startErr: errors.New("stopped")}
	wg := &fakeWorkerGroup{}
	pool := newTestPool(t, brk, wg, nil)

	// Stop before Start must be a no-op rather than a panic or a deadlock.
	assert.NotPanics(t, func() { pool.Stop() })
	assert.Equal(t, int64(0), brk.stopCalls.Load())

	go func() { _ = pool.Start() }()
	require.Eventually(t, func() bool {
		return brk.startCalls.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	assert.NotPanics(t, func() {
		pool.Stop()
		pool.Stop()
	})
	assert.Equal(t, int64(1), brk.stopCalls.Load(), "the broker must only be stopped once")
}

func TestStartIsIdempotent(t *testing.T) {
	brk := &fakeBroker{startErr: errors.New("stopped")}
	pool := newTestPool(t, brk, &fakeWorkerGroup{}, nil)

	go func() { _ = pool.Start() }()
	require.Eventually(t, func() bool {
		return brk.startCalls.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	// A second Start must not spawn a second supervisor.
	assert.NoError(t, pool.Start())

	pool.Stop()
}

// TestConsumerIsRestartedAfterBrokerFailure is the reconnect guarantee: losing
// RabbitMQ must not silently leave the pool without a consumer.
func TestConsumerIsRestartedAfterBrokerFailure(t *testing.T) {
	brk := &fakeBroker{startErr: errors.New("connection reset by peer")}
	pool := newTestPool(t, brk, &fakeWorkerGroup{}, nil)

	go func() { _ = pool.Start() }()

	require.Eventually(t, func() bool {
		return brk.startCalls.Load() >= 3
	}, 15*time.Second, 50*time.Millisecond, "consumer was not restarted after failures")

	pool.Stop()

	// No further restarts once the pool is stopped.
	calls := brk.startCalls.Load()
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, calls, brk.startCalls.Load())
}

func TestReconnectBackoffIsBounded(t *testing.T) {
	assert.Greater(t, reconnectMaxDelay, reconnectInitialDelay)

	for _, d := range []time.Duration{
		reconnectInitialDelay,
		time.Second,
		reconnectMaxDelay,
	} {
		jittered := jitterDuration(d)
		assert.GreaterOrEqual(t, jittered, d/2)
		assert.Less(t, jittered, d)
	}

	assert.Equal(t, time.Duration(0), jitterDuration(0))
	assert.Equal(t, time.Duration(0), jitterDuration(-time.Second))
	// A duration too small to halve is returned unchanged instead of panicking
	// inside rand.Int63n.
	assert.NotPanics(t, func() { jitterDuration(1) })
}

func TestSleepOrStopReturnsEarlyOnStop(t *testing.T) {
	pool := newTestPool(t, &fakeBroker{}, &fakeWorkerGroup{}, nil)

	result := make(chan bool, 1)
	go func() { result <- pool.sleepOrStop(30 * time.Second) }()

	time.Sleep(50 * time.Millisecond)
	close(pool.stopChan)

	select {
	case completed := <-result:
		assert.False(t, completed, "sleepOrStop must report that it was interrupted")
	case <-time.After(5 * time.Second):
		t.Fatal("sleepOrStop ignored the stop signal")
	}
}

func TestIsStopping(t *testing.T) {
	pool := newTestPool(t, &fakeBroker{}, &fakeWorkerGroup{}, nil)

	// Not started yet: the pool counts as stopping so no consumer is spawned.
	assert.True(t, pool.isStopping())

	pool.started.Store(true)
	assert.False(t, pool.isStopping())

	close(pool.stopChan)
	assert.True(t, pool.isStopping())
}

// --- task API ---------------------------------------------------------------

func TestSendTaskWithContext(t *testing.T) {
	pool := newTestPool(t, &fakeBroker{}, &fakeWorkerGroup{}, &fakeRegistrar{})

	state, err := pool.SendTaskWithContext(context.Background(), &schema.Signature{UUID: "task_1"})

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Pending", state.Status)
	assert.Equal(t, "task_1", state.Request.UUID)
}

func TestSendTaskWithContextWrapsBrokerError(t *testing.T) {
	sendErr := errors.New("broker unavailable")
	pool := newTestPool(t, &fakeBroker{}, &fakeWorkerGroup{}, &fakeRegistrar{sendErr: sendErr})

	state, err := pool.SendTaskWithContext(context.Background(), &schema.Signature{})

	assert.Nil(t, state)
	// The cause must stay inspectable with errors.Is for retry decisions.
	assert.ErrorIs(t, err, sendErr)
}

func TestBrokerAndBackendAccessors(t *testing.T) {
	brk := &fakeBroker{}
	pool := newTestPool(t, brk, &fakeWorkerGroup{}, nil)

	assert.Equal(t, brk, pool.GetBroker())

	replacement := &fakeBroker{}
	pool.SetBroker(replacement)
	assert.Equal(t, replacement, pool.GetBroker())

	assert.Nil(t, pool.GetBackend())
	pool.SetBackend(nil)
	assert.Nil(t, pool.GetBackend())
}

var _ factory.IFactory = &fakeFactory{}
