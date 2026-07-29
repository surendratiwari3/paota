package amqp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/schema"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

// capturingProvider records what was published without touching a real broker.
type capturingProvider struct {
	provider.AmqpProviderInterface

	mu           sync.Mutex
	routingKeys  []string
	exchanges    []string
	messages     []amqp.Publishing
	publishError error
}

func (c *capturingProvider) AmqpPublishWithConfirm(_ context.Context, routingKey string, msg amqp.Publishing, exchange string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.publishError != nil {
		return c.publishError
	}
	c.routingKeys = append(c.routingKeys, routingKey)
	c.exchanges = append(c.exchanges, exchange)
	c.messages = append(c.messages, msg)
	return nil
}

func (c *capturingProvider) lastPublish() (string, amqp.Publishing) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.messages) - 1
	return c.routingKeys[n], c.messages[n]
}

func testBroker(cfg *config.Config) (*AMQPBroker, *capturingProvider) {
	prov := &capturingProvider{}
	return &AMQPBroker{
		config:          cfg,
		amqpProvider:    prov,
		stopChannel:     make(chan struct{}, 1),
		doneStopChannel: make(chan struct{}, 1),
	}, prov
}

func brokerConfig() *config.Config {
	return &config.Config{
		Broker:        "amqp",
		TaskQueueName: "paota_tasks",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			DelayedQueue:       "paota_task_delayed_queue",
			FailedQueue:        "paota_task_failed_queue",
			TimeoutQueue:       "paota_task_timeout_queue",
			PrefetchCount:      10,
			ConnectionPoolSize: 2,
		},
	}
}

// --- construction -----------------------------------------------------------

// TestNewAMQPBrokerRejectsMissingConfig covers a nil dereference on start-up.
func TestNewAMQPBrokerRejectsMissingConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider config.ConfigProvider
	}{
		{"nil provider", nil},
		{"nil config", &stubConfigProvider{cfg: nil}},
		{"nil amqp section", &stubConfigProvider{cfg: &config.Config{Broker: "amqp"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				brk, err := NewAMQPBroker(tt.provider)
				assert.Nil(t, brk)
				assert.ErrorIs(t, err, appErrors.ErrNilConfig)
			})
		})
	}
}

type stubConfigProvider struct{ cfg *config.Config }

func (s *stubConfigProvider) ReadFromEnv() error                       { return nil }
func (s *stubConfigProvider) SetApplicationConfig(config.Config) error { return nil }
func (s *stubConfigProvider) ValidateConfig(config.Config) error       { return nil }
func (s *stubConfigProvider) GetConfig() *config.Config                { return s.cfg }

// --- routing ----------------------------------------------------------------

func TestGetRoutingKey(t *testing.T) {
	tests := []struct {
		name         string
		exchangeType string
		bindingKey   string
		expected     string
	}{
		{"direct exchange uses binding key", "direct", "paota_task_binding_key", "paota_task_binding_key"},
		{"topic exchange uses queue name", "topic", "paota_task_binding_key", "paota_tasks"},
		{"empty binding key uses queue name", "direct", "", "paota_tasks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := brokerConfig()
			cfg.AMQP.ExchangeType = tt.exchangeType
			cfg.AMQP.BindingKey = tt.bindingKey
			b, _ := testBroker(cfg)

			assert.Equal(t, tt.expected, b.getRoutingKey())
		})
	}
}

// TestGetRoutingKeyWithNilAmqpConfig covers a nil dereference: the nil check
// lived in isDirectExchange but getRoutingKey read BindingKey first.
func TestGetRoutingKeyWithNilAmqpConfig(t *testing.T) {
	b, _ := testBroker(&config.Config{TaskQueueName: "paota_tasks"})

	assert.NotPanics(t, func() {
		assert.Equal(t, "paota_tasks", b.getRoutingKey())
	})
}

// --- publishing -------------------------------------------------------------

func TestPublishUsesDefaultRoutingKey(t *testing.T) {
	b, prov := testBroker(brokerConfig())

	require.NoError(t, b.Publish(context.Background(), &schema.Signature{Name: "sendEmail"}))

	routingKey, msg := prov.lastPublish()
	assert.Equal(t, "paota_task_binding_key", routingKey)
	assert.Equal(t, amqp.Persistent, msg.DeliveryMode, "tasks must survive a broker restart")
}

func TestPublishHonoursExplicitRoutingKey(t *testing.T) {
	b, prov := testBroker(brokerConfig())

	require.NoError(t, b.Publish(context.Background(), &schema.Signature{
		Name:       "sendEmail",
		RoutingKey: "custom_queue",
	}))

	routingKey, _ := prov.lastPublish()
	assert.Equal(t, "custom_queue", routingKey)
}

// TestPublishDelayedMessageStillHasRoutingKey covers messages published into
// the void: with an ETA set and no explicit routing key, the routing key
// defaulting was skipped and the broker dropped the message.
func TestPublishDelayedMessageStillHasRoutingKey(t *testing.T) {
	b, prov := testBroker(brokerConfig())
	eta := time.Now().UTC().Add(30 * time.Second)

	require.NoError(t, b.Publish(context.Background(), &schema.Signature{
		Name: "sendEmail",
		ETA:  &eta,
	}))

	routingKey, msg := prov.lastPublish()
	assert.NotEmpty(t, routingKey, "a delayed message must still be routable")
	assert.Equal(t, "paota_task_binding_key", routingKey)
	assert.NotEmpty(t, msg.Expiration, "a delayed message must carry a TTL")
}

func TestPublishSerialisesFullSignatureWhenNoRawArgs(t *testing.T) {
	b, prov := testBroker(brokerConfig())

	require.NoError(t, b.Publish(context.Background(), &schema.Signature{
		UUID: "task_1",
		Name: "sendEmail",
	}))

	_, msg := prov.lastPublish()
	var decoded schema.Signature
	require.NoError(t, json.Unmarshal(msg.Body, &decoded))
	assert.Equal(t, "sendEmail", decoded.Name)
	assert.Equal(t, "task_1", decoded.UUID)
}

func TestPublishUsesRawArgsAsBody(t *testing.T) {
	b, prov := testBroker(brokerConfig())
	raw := json.RawMessage(`{"to":"someone@example.com"}`)

	require.NoError(t, b.Publish(context.Background(), &schema.Signature{
		UUID:    "task_1",
		Name:    "sendEmail",
		RawArgs: raw,
	}))

	_, msg := prov.lastPublish()
	assert.JSONEq(t, string(raw), string(msg.Body))
	assert.Equal(t, "sendEmail", msg.Headers["task_name"])
}

func TestPublishRejectsNilSignature(t *testing.T) {
	b, _ := testBroker(brokerConfig())

	assert.NotPanics(t, func() {
		assert.Error(t, b.Publish(context.Background(), nil))
	})
}

// TestBuildHeadersRoundTrip is the header contract between publisher and
// consumer: every field the consumer reads must actually be written.
func TestBuildHeadersRoundTrip(t *testing.T) {
	createdAt := time.Now().UTC().Truncate(time.Second)
	eta := createdAt.Add(time.Minute)
	sig := &schema.Signature{
		UUID:                        "task_1",
		Name:                        "sendEmail",
		RoutingKey:                  "paota_tasks",
		Priority:                    7,
		RetryCount:                  3,
		RetryTimeout:                60,
		WaitTime:                    2,
		RetriesDone:                 1,
		TaskTimeOut:                 30,
		IgnoreWhenTaskNotRegistered: true,
		CreatedAt:                   &createdAt,
		ETA:                         &eta,
	}

	headers := buildHeadersFromSignature(sig)

	// The headers have to be encodable by amqp091 or the publish fails at
	// runtime.
	require.NoError(t, headers.Validate())

	assert.Equal(t, "task_1", headers["uuid"])
	assert.Equal(t, "sendEmail", headers["task_name"])
	assert.Equal(t, 3, headers["retry_count"])
	assert.Equal(t, 60, headers["retry_timeout"])
	assert.Equal(t, 30, headers["timeout"])
	assert.Equal(t, 2, headers["wait_time"])
	assert.Equal(t, 1, headers["retries_done"])
	assert.Equal(t, true, headers["ignore_if_unregistered"])
	assert.Equal(t, createdAt.Unix(), headers["created_at"])
	assert.Equal(t, eta.Unix(), headers["eta"])
	// These two were read by the consumer but never written by the publisher.
	assert.Equal(t, "paota_tasks", headers["routing_key"])
	assert.Equal(t, 7, headers["priority"])
}

func TestBuildHeadersOmitsUnsetTimestamps(t *testing.T) {
	headers := buildHeadersFromSignature(&schema.Signature{Name: "sendEmail"})

	require.NoError(t, headers.Validate())
	_, hasCreatedAt := headers["created_at"]
	_, hasETA := headers["eta"]
	assert.False(t, hasCreatedAt)
	assert.False(t, hasETA)
}

// --- delay calculation ------------------------------------------------------

func TestGetTaskTTL(t *testing.T) {
	b, _ := testBroker(brokerConfig())
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(10 * time.Second)

	assert.Equal(t, int64(0), b.getTaskTTL(&schema.Signature{}), "no ETA means no delay")
	assert.Equal(t, int64(0), b.getTaskTTL(&schema.Signature{ETA: &past}), "an ETA in the past means no delay")

	ttl := b.getTaskTTL(&schema.Signature{ETA: &future})
	assert.Greater(t, ttl, int64(0))
	assert.LessOrEqual(t, ttl, int64(10_000))

	// High priority tasks with a deadline jump the delay queue.
	assert.Equal(t, int64(5), b.getTaskTTL(&schema.Signature{
		ETA:         &future,
		Priority:    5,
		TaskTimeOut: 30,
		CreatedAt:   &now,
	}))
}

// --- lifecycle --------------------------------------------------------------

// TestStopConsumerWithoutConsumer covers a guaranteed deadlock: StopConsumer
// used to block forever on an unbuffered channel nobody was reading.
func TestStopConsumerWithoutConsumer(t *testing.T) {
	b, _ := testBroker(brokerConfig())

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.StopConsumer()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopConsumer deadlocked when no consumer was running")
	}
}

func TestStopConsumerIsIdempotent(t *testing.T) {
	b, _ := testBroker(brokerConfig())

	assert.NotPanics(t, func() {
		b.StopConsumer()
		b.StopConsumer()
	})
}

func TestStartConsumerRejectsNilWorkerGroup(t *testing.T) {
	b, _ := testBroker(brokerConfig())

	err := b.StartConsumer(context.Background(), nil)

	assert.Error(t, err)
	assert.False(t, b.consuming.Load())
}

func TestConnectionErrorIsNeverNil(t *testing.T) {
	// NotifyClose delivers a nil *amqp.Error on a graceful close; the caller
	// relies on a non-nil error meaning "restart the consumer".
	assert.Error(t, connectionError(nil, "amqp channel closed"))
	assert.EqualError(t, connectionError(nil, "amqp channel closed"), "amqp channel closed")

	amqpErr := &amqp.Error{Code: 320, Reason: "connection closed"}
	assert.ErrorIs(t, connectionError(amqpErr, "ignored"), amqpErr)
}

func TestWaitTimeout(t *testing.T) {
	var wg sync.WaitGroup
	assert.True(t, waitTimeout(&wg, time.Second), "an empty wait group returns immediately")

	wg.Add(1)
	assert.False(t, waitTimeout(&wg, 50*time.Millisecond), "a blocked wait group must time out")
	wg.Done()
}
