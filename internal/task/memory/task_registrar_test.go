package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/schema"
	appError "github.com/surendratiwari3/paota/schema/errors"
)

// --- test doubles -----------------------------------------------------------

// stubProvider is a hand written config provider so these tests do not depend
// on mockery generated mocks.
type stubProvider struct{ cfg *config.Config }

func (s *stubProvider) ReadFromEnv() error                       { return nil }
func (s *stubProvider) SetApplicationConfig(config.Config) error { return nil }
func (s *stubProvider) ValidateConfig(config.Config) error       { return nil }
func (s *stubProvider) GetConfig() *config.Config                { return s.cfg }

// recordingBroker captures published signatures.
type recordingBroker struct {
	mu        sync.Mutex
	published []schema.Signature
	err       error
}

func (b *recordingBroker) StartConsumer(context.Context, workergroup.WorkerGroupInterface) error {
	return nil
}

func (b *recordingBroker) StopConsumer() {}

func (b *recordingBroker) Publish(_ context.Context, sig *schema.Signature) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	b.published = append(b.published, *sig)
	return nil
}

func (b *recordingBroker) last() schema.Signature {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.published[len(b.published)-1]
}

func (b *recordingBroker) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.published)
}

// fakeAcknowledger records how a delivery was settled, which is what decides
// whether a message is kept, redelivered or dropped by RabbitMQ.
type fakeAcknowledger struct {
	acks     atomic.Int64
	nacks    atomic.Int64
	requeued atomic.Bool
	rejects  atomic.Int64
}

func (f *fakeAcknowledger) Ack(uint64, bool) error { f.acks.Add(1); return nil }
func (f *fakeAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	f.nacks.Add(1)
	f.requeued.Store(requeue)
	return nil
}
func (f *fakeAcknowledger) Reject(uint64, bool) error { f.rejects.Add(1); return nil }

func newDelivery(ack *fakeAcknowledger, headers amqp.Table, body []byte) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: ack,
		Headers:      headers,
		Body:         body,
		DeliveryTag:  1,
	}
}

func testConfig() *config.Config {
	return &config.Config{
		Broker:                   "amqp",
		TaskQueueName:            "paota_tasks",
		MaxTaskExecutionDuration: 10,
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			DelayedQueue:       "paota_task_delayed_queue",
			FailedQueue:        "paota_task_failed_queue",
			TimeoutQueue:       "paota_task_timeout_queue",
			ConnectionPoolSize: 2,
		},
	}
}

func newRegistrar(t *testing.T, cfg *config.Config) (*DefaultTaskRegistrar, *recordingBroker) {
	t.Helper()
	brk := &recordingBroker{}
	registrar := &DefaultTaskRegistrar{
		registeredTasks: new(sync.Map),
		broker:          brk,
		configProvider:  &stubProvider{cfg: cfg},
	}
	return registrar, brk
}

// --- registration -----------------------------------------------------------

// TestRegisterTasksCountsEachNameOnce covers a counter that was incremented on
// every call, so re-registering the same task inflated the count forever.
func TestRegisterTasksCountsEachNameOnce(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	tasks := map[string]interface{}{
		"a": func() error { return nil },
		"b": func() error { return nil },
	}

	require.NoError(t, registrar.RegisterTasks(tasks))
	assert.Equal(t, uint(2), registrar.GetRegisteredTaskCount())

	require.NoError(t, registrar.RegisterTasks(tasks))
	assert.Equal(t, uint(2), registrar.GetRegisteredTaskCount())

	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{"c": func() error { return nil }}))
	assert.Equal(t, uint(3), registrar.GetRegisteredTaskCount())
}

func TestRegisterTasksRejectsInvalidTask(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	err := registrar.RegisterTasks(map[string]interface{}{"bad": "not a function"})

	assert.Error(t, err)
	assert.False(t, registrar.IsTaskRegistered("bad"))
	assert.Equal(t, uint(0), registrar.GetRegisteredTaskCount())
}

// TestRegisterTasksIsAtomicOnFailure: one invalid task must not leave the
// registry half populated.
func TestRegisterTasksIsAtomicOnFailure(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	err := registrar.RegisterTasks(map[string]interface{}{
		"good": func() error { return nil },
		"bad":  func() {},
	})

	assert.Error(t, err)
	assert.False(t, registrar.IsTaskRegistered("good"))
}

// TestConcurrentRegistrationAndCount runs under -race: the counter used to be a
// plain uint written while other goroutines read it.
func TestConcurrentRegistrationAndCount(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := string(rune('a' + i))
			_ = registrar.RegisterTasks(map[string]interface{}{name: func() error { return nil }})
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registrar.GetRegisteredTaskCount()
		}()
	}
	wg.Wait()

	assert.Equal(t, uint(20), registrar.GetRegisteredTaskCount())
}

func TestGetRegisteredTask(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{"known": func() error { return nil }}))

	fn, err := registrar.GetRegisteredTask("known")
	require.NoError(t, err)
	assert.NotNil(t, fn)

	fn, err = registrar.GetRegisteredTask("unknown")
	assert.Error(t, err)
	assert.Nil(t, fn)
}

// --- sending ----------------------------------------------------------------

func TestSendTaskFillsInDefaults(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())

	sig := &schema.Signature{Name: "sendEmail"}
	require.NoError(t, registrar.SendTask(sig))

	published := brk.last()
	assert.NotEmpty(t, published.UUID)
	assert.Contains(t, published.UUID, "task_")
	require.NotNil(t, published.CreatedAt)
	assert.WithinDuration(t, time.Now().UTC(), *published.CreatedAt, 5*time.Second)
}

func TestSendTaskKeepsExplicitValues(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, registrar.SendTask(&schema.Signature{
		Name:      "sendEmail",
		UUID:      "task_explicit",
		CreatedAt: &createdAt,
	}))

	published := brk.last()
	assert.Equal(t, "task_explicit", published.UUID)
	assert.True(t, createdAt.Equal(*published.CreatedAt))
}

func TestSendTaskPropagatesBrokerError(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	brk.err = errors.New("broker unavailable")

	err := registrar.SendTaskWithContext(context.Background(), &schema.Signature{Name: "sendEmail"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "broker unavailable")
}

func TestSendTaskRejectsNilSignature(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	assert.NotPanics(t, func() {
		assert.Error(t, registrar.SendTask(nil))
	})
}

// --- retry policy -----------------------------------------------------------

func TestRetryTaskSchedulesRetryOnDelayedQueue(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	sig := &schema.Signature{Name: "sendEmail", UUID: "task_1", RetryCount: 3, RetriesDone: 0}

	require.NoError(t, registrar.retryTask(sig))

	published := brk.last()
	assert.Equal(t, "paota_task_delayed_queue", published.RoutingKey)
	assert.Equal(t, 1, published.RetriesDone)
	require.NotNil(t, published.ETA)
	assert.True(t, published.ETA.After(time.Now().UTC()))
}

// TestRetryTaskAlwaysAdvancesRetriesDone guards against an infinite retry loop.
func TestRetryTaskAlwaysAdvancesRetriesDone(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	sig := &schema.Signature{Name: "sendEmail", UUID: "task_1", RetryCount: 3}

	for i := 1; i <= 3; i++ {
		require.NoError(t, registrar.retryTask(sig))
		assert.Equal(t, i, sig.RetriesDone)
	}

	// Retries are exhausted: the task must be parked, not retried again.
	require.NoError(t, registrar.retryTask(sig))
	assert.Equal(t, "paota_task_failed_queue", brk.last().RoutingKey)
}

func TestRetryTaskParksExhaustedTask(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	eta := time.Now().Add(time.Hour)
	sig := &schema.Signature{Name: "sendEmail", RetryCount: 2, RetriesDone: 2, ETA: &eta}

	require.NoError(t, registrar.retryTask(sig))

	published := brk.last()
	assert.Equal(t, "paota_task_failed_queue", published.RoutingKey)
	// A stale ETA would have RabbitMQ hold the task in the delay queue instead
	// of delivering it to the failed queue.
	assert.Nil(t, published.ETA)
}

func TestRetryTaskWithoutRetriesConfigured(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())

	require.NoError(t, registrar.retryTask(&schema.Signature{Name: "sendEmail", RetryCount: 0}))

	assert.Equal(t, "paota_task_failed_queue", brk.last().RoutingKey)
}

func TestRetryTaskWithoutFailedQueueIsNoop(t *testing.T) {
	cfg := testConfig()
	cfg.AMQP.FailedQueue = ""
	registrar, brk := newRegistrar(t, cfg)

	require.NoError(t, registrar.retryTask(&schema.Signature{Name: "sendEmail", RetryCount: 0}))

	assert.Equal(t, 0, brk.count())
}

// TestRetryTaskWithNilAmqpConfig covers a nil dereference on a partially
// configured worker.
func TestRetryTaskWithNilAmqpConfig(t *testing.T) {
	cfg := testConfig()
	cfg.AMQP = nil
	registrar, _ := newRegistrar(t, cfg)

	assert.NotPanics(t, func() {
		assert.NoError(t, registrar.retryTask(&schema.Signature{Name: "sendEmail"}))
	})
}

func TestGetRetryIntervalIsCapped(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	assert.Equal(t, 1*time.Second, registrar.getRetryInterval(1))
	assert.Equal(t, 1*time.Second, registrar.getRetryInterval(2))
	assert.Equal(t, 2*time.Second, registrar.getRetryInterval(3))
	assert.Equal(t, 3*time.Second, registrar.getRetryInterval(4))
	assert.Equal(t, 5*time.Second, registrar.getRetryInterval(5))
	assert.Equal(t, 8*time.Second, registrar.getRetryInterval(6))
	assert.Equal(t, maxRetryInterval, registrar.getRetryInterval(7))

	// A heavily retried task must not spin for minutes computing its backoff.
	done := make(chan time.Duration, 1)
	go func() { done <- registrar.getRetryInterval(60) }()
	select {
	case d := <-done:
		assert.Equal(t, maxRetryInterval, d)
	case <-time.After(2 * time.Second):
		t.Fatal("getRetryInterval(60) hung")
	}
}

// --- timeout handling -------------------------------------------------------

func TestCheckTaskTimeout(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	old := time.Now().UTC().Add(-time.Hour)
	fresh := time.Now().UTC()

	assert.True(t, registrar.checkTaskTimeout(&schema.Signature{TaskTimeOut: 30, CreatedAt: &old}))
	assert.False(t, registrar.checkTaskTimeout(&schema.Signature{TaskTimeOut: 30, CreatedAt: &fresh}))
	assert.False(t, registrar.checkTaskTimeout(&schema.Signature{TaskTimeOut: 0, CreatedAt: &old}))
	assert.False(t, registrar.checkTaskTimeout(&schema.Signature{TaskTimeOut: 30}))
}

func TestPushToTimeoutQueue(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	eta := time.Now().Add(time.Hour)
	sig := &schema.Signature{Name: "sendEmail", TaskTimeOut: 30, ETA: &eta}

	require.NoError(t, registrar.pushToTimeoutAmqpQueue(sig))

	published := brk.last()
	assert.Equal(t, "paota_task_timeout_queue", published.RoutingKey)
	// Cleared so the task is not timed out again while it is being handled
	// from the timeout queue.
	assert.Equal(t, 0, published.TaskTimeOut)
	assert.Nil(t, published.ETA)
}

func TestPushToTimeoutQueueWithoutQueueConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.AMQP.TimeoutQueue = ""
	registrar, brk := newRegistrar(t, cfg)

	require.NoError(t, registrar.pushToTimeoutAmqpQueue(&schema.Signature{Name: "sendEmail"}))

	assert.Equal(t, 0, brk.count())
}

// TestMaxTaskExecutionDurationFallback covers a zero configuration value, which
// produced an already-expired context so every task was reported as timed out.
func TestMaxTaskExecutionDurationFallback(t *testing.T) {
	cfg := testConfig()
	cfg.MaxTaskExecutionDuration = 0
	registrar, _ := newRegistrar(t, cfg)

	assert.Equal(t, defaultMaxTaskExecutionDuration, registrar.maxTaskExecutionDuration())

	cfg.MaxTaskExecutionDuration = 25
	assert.Equal(t, 25*time.Second, registrar.maxTaskExecutionDuration())
}

func TestGetConfigHelpersNeverReturnNil(t *testing.T) {
	registrar := &DefaultTaskRegistrar{registeredTasks: new(sync.Map)}

	assert.NotNil(t, registrar.getConfig())
	assert.NotNil(t, registrar.getAMQPConfig())

	registrar.configProvider = &stubProvider{cfg: nil}
	assert.NotNil(t, registrar.getConfig())
	assert.NotNil(t, registrar.getAMQPConfig())
}

// --- header decoding --------------------------------------------------------

func TestSignatureFromAmqpHeadersAndBody(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	createdAt := time.Now().UTC().Truncate(time.Second)
	eta := createdAt.Add(time.Minute)

	// These are the wire types RabbitMQ actually delivers.
	headers := amqp.Table{
		"task_name":              "sendEmail",
		"uuid":                   "task_1",
		"routing_key":            "paota_tasks",
		"priority":               int32(5),
		"retry_count":            int32(3),
		"retry_timeout":          int32(60),
		"wait_time":              int32(2),
		"retries_done":           int32(1),
		"timeout":                int32(30),
		"ignore_if_unregistered": true,
		"created_at":             createdAt.Unix(),
		"eta":                    eta.Unix(),
	}
	body := []byte(`{"to":"someone@example.com"}`)

	sig, err := registrar.SignatureFromAmqpHeadersAndBody(headers, body)

	require.NoError(t, err)
	assert.Equal(t, "sendEmail", sig.Name)
	assert.Equal(t, "task_1", sig.UUID)
	assert.Equal(t, "paota_tasks", sig.RoutingKey)
	// Priority came back as 0 before the header decoder learned about int32.
	assert.Equal(t, uint8(5), sig.Priority)
	assert.Equal(t, 3, sig.RetryCount)
	assert.Equal(t, 60, sig.RetryTimeout)
	assert.Equal(t, 2, sig.WaitTime)
	assert.Equal(t, 1, sig.RetriesDone)
	assert.Equal(t, 30, sig.TaskTimeOut)
	assert.True(t, sig.IgnoreWhenTaskNotRegistered)
	assert.Equal(t, json.RawMessage(body), sig.RawArgs)
	require.NotNil(t, sig.CreatedAt)
	assert.True(t, createdAt.Equal(*sig.CreatedAt))
	require.NotNil(t, sig.ETA)
	assert.True(t, eta.Equal(*sig.ETA))
}

func TestSignatureFromAmqpHeadersWithMissingHeaders(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	sig, err := registrar.SignatureFromAmqpHeadersAndBody(amqp.Table{"task_name": "sendEmail"}, []byte(`{}`))

	require.NoError(t, err)
	assert.Equal(t, "sendEmail", sig.Name)
	assert.Equal(t, 0, sig.RetryCount)
	assert.Nil(t, sig.CreatedAt)
	assert.Nil(t, sig.ETA)
}

// --- message processing -----------------------------------------------------

func TestProcessorAcksSuccessfulTask(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	var ran atomic.Bool
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { ran.Store(true); return nil },
	}))

	ack := &fakeAcknowledger{}
	require.NoError(t, registrar.Processor(newDelivery(ack,
		amqp.Table{"task_name": "sendEmail"}, []byte(`{"to":"x"}`))))

	assert.True(t, ran.Load())
	assert.Equal(t, int64(1), ack.acks.Load())
	assert.Equal(t, int64(0), ack.nacks.Load())
}

func TestProcessorRetriesFailedTask(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { return errors.New("smtp down") },
	}))

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack,
		amqp.Table{"task_name": "sendEmail", "retry_count": int32(3)}, []byte(`{}`)))

	require.NoError(t, err)
	// The original message is acked only once its replacement is safely
	// published, so the task is never lost.
	assert.Equal(t, int64(1), ack.acks.Load())
	assert.Equal(t, 1, brk.count())
	assert.Equal(t, "paota_task_delayed_queue", brk.last().RoutingKey)
}

// TestProcessorRequeuesWhenRetryPublishFails is the no-message-loss guarantee:
// if the retry cannot be published, the delivery must go back to the queue.
func TestProcessorRequeuesWhenRetryPublishFails(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	brk.err = errors.New("broker unavailable")
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { return errors.New("smtp down") },
	}))

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack,
		amqp.Table{"task_name": "sendEmail", "retry_count": int32(3)}, []byte(`{}`)))

	assert.Error(t, err)
	assert.Equal(t, int64(0), ack.acks.Load())
	assert.Equal(t, int64(1), ack.nacks.Load())
	assert.True(t, ack.requeued.Load(), "the delivery must be requeued, not dropped")
}

// TestProcessorSurvivesPanickingTask: a task that panics must not take the
// worker down, and the message must still be settled.
func TestProcessorSurvivesPanickingTask(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { panic("boom") },
	}))

	ack := &fakeAcknowledger{}
	assert.NotPanics(t, func() {
		_ = registrar.Processor(newDelivery(ack,
			amqp.Table{"task_name": "sendEmail", "retry_count": int32(3)}, []byte(`{}`)))
	})

	assert.Equal(t, 1, brk.count(), "a panicking task must still be retried")
	assert.Equal(t, int64(1), ack.acks.Load())
}

func TestProcessorTimesOutLongRunningTask(t *testing.T) {
	cfg := testConfig()
	cfg.MaxTaskExecutionDuration = 1
	registrar, brk := newRegistrar(t, cfg)

	release := make(chan struct{})
	defer close(release)
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"slow": func(*schema.Signature) error { <-release; return nil },
	}))

	ack := &fakeAcknowledger{}
	start := time.Now()
	err := registrar.Processor(newDelivery(ack,
		amqp.Table{"task_name": "slow", "retry_count": int32(3)}, []byte(`{}`)))

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, 1, brk.count(), "a timed out task must be rescheduled")
	assert.Equal(t, int64(1), ack.acks.Load())
}

func TestProcessorParksTaskThatWaitedTooLong(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { return nil },
	}))

	createdAt := time.Now().UTC().Add(-time.Hour)
	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, amqp.Table{
		"task_name":  "sendEmail",
		"timeout":    int32(30),
		"created_at": createdAt.Unix(),
	}, []byte(`{}`)))

	require.NoError(t, err)
	assert.Equal(t, "paota_task_timeout_queue", brk.last().RoutingKey)
	assert.Equal(t, int64(1), ack.acks.Load())
}

func TestProcessorRejectsEmptyMessage(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, amqp.Table{"task_name": "sendEmail"}, nil))

	assert.ErrorIs(t, err, appError.ErrEmptyMessage)
	assert.Equal(t, int64(1), ack.nacks.Load())
	assert.False(t, ack.requeued.Load(), "a permanently bad message must not be requeued")
}

func TestProcessorRejectsUndecodableMessage(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, nil, []byte("this is not json")))

	assert.Error(t, err)
	assert.Equal(t, int64(1), ack.nacks.Load())
}

// TestProcessorParksUnregisteredTask covers silent data loss: an unknown task
// used to be nacked without requeue and disappeared.
func TestProcessorParksUnregisteredTask(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, amqp.Table{"task_name": "unknown"}, []byte(`{}`)))

	assert.ErrorIs(t, err, appError.ErrTaskNotRegistered)
	assert.Equal(t, 1, brk.count())
	assert.Equal(t, "paota_task_failed_queue", brk.last().RoutingKey)
	assert.Equal(t, int64(1), ack.acks.Load())
}

func TestProcessorIgnoresUnregisteredTaskWhenAsked(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, amqp.Table{
		"task_name":              "unknown",
		"ignore_if_unregistered": true,
	}, []byte(`{}`)))

	assert.ErrorIs(t, err, appError.ErrTaskNotRegistered)
	assert.Equal(t, 0, brk.count())
	assert.Equal(t, int64(1), ack.acks.Load())
}

func TestProcessorRequeuesUnregisteredTaskWhenParkingFails(t *testing.T) {
	registrar, brk := newRegistrar(t, testConfig())
	brk.err = errors.New("broker unavailable")

	ack := &fakeAcknowledger{}
	_ = registrar.Processor(newDelivery(ack, amqp.Table{"task_name": "unknown"}, []byte(`{}`)))

	assert.Equal(t, int64(1), ack.nacks.Load())
	assert.True(t, ack.requeued.Load())
}

func TestProcessorRejectsWrongTaskSignature(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	// Registered, but not with the signature the processor can invoke.
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func() error { return nil },
	}))

	ack := &fakeAcknowledger{}
	err := registrar.Processor(newDelivery(ack, amqp.Table{"task_name": "sendEmail"}, []byte(`{}`)))

	assert.ErrorIs(t, err, appError.ErrTaskMustBeFunc)
	assert.Equal(t, int64(1), ack.nacks.Load())
}

func TestProcessorIgnoresUnsupportedJobType(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())

	assert.NotPanics(t, func() {
		assert.NoError(t, registrar.Processor("not a delivery"))
	})
}

func TestProcessorHandlesFullSignatureBody(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	var ran atomic.Bool
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { ran.Store(true); return nil },
	}))

	body, err := json.Marshal(&schema.Signature{UUID: "task_1", Name: "sendEmail"})
	require.NoError(t, err)

	ack := &fakeAcknowledger{}
	// No task_name header: the whole signature travels in the body.
	require.NoError(t, registrar.Processor(newDelivery(ack, nil, body)))

	assert.True(t, ran.Load())
	assert.Equal(t, int64(1), ack.acks.Load())
}

// TestConcurrentProcessing checks the registrar is safe to share across the
// worker goroutines that all call into it at once.
func TestConcurrentProcessing(t *testing.T) {
	registrar, _ := newRegistrar(t, testConfig())
	var processed atomic.Int64
	require.NoError(t, registrar.RegisterTasks(map[string]interface{}{
		"sendEmail": func(*schema.Signature) error { processed.Add(1); return nil },
	}))

	const messages = 100
	var wg sync.WaitGroup
	acks := make([]*fakeAcknowledger, messages)
	for i := 0; i < messages; i++ {
		acks[i] = &fakeAcknowledger{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = registrar.Processor(newDelivery(acks[i],
				amqp.Table{"task_name": "sendEmail"}, []byte(`{}`)))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(messages), processed.Load())
	for i, ack := range acks {
		assert.Equal(t, int64(1), ack.acks.Load(), "message %d was not acked exactly once", i)
	}
}
