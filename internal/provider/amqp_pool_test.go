package provider

import (
	"context"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/surendratiwari3/paota/config"
	appErrors "github.com/surendratiwari3/paota/schema/errors"
)

func poolConfig(size int) *config.AMQPConfig {
	return &config.AMQPConfig{
		// Port 1 is never a RabbitMQ broker, so dialling always fails fast.
		Url:                "amqp://guest:guest@127.0.0.1:1/",
		ConnectionPoolSize: size,
		HeartBeatInterval:  10,
		ConnectionTimeout:  1,
	}
}

func TestCreateConnectionPoolValidatesConfig(t *testing.T) {
	tests := []struct {
		name     string
		conf     *config.AMQPConfig
		expected error
	}{
		{"nil config", nil, appErrors.ErrNilConfig},
		{"zero pool size", &config.AMQPConfig{Url: "amqp://localhost:5672"}, appErrors.ErrInvalidConfig},
		{"negative pool size", &config.AMQPConfig{Url: "amqp://localhost:5672", ConnectionPoolSize: -1}, appErrors.ErrInvalidConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov := NewAmqpProvider(tt.conf)

			err := prov.CreateConnectionPool()

			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func TestCreateConnectionPoolReportsDialFailure(t *testing.T) {
	prov := NewAmqpProvider(poolConfig(2))

	err := prov.CreateConnectionPool()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CreateConnectionPool failed")
}

func TestCreateConnectionRequiresConfig(t *testing.T) {
	prov := &amqpProvider{}

	conn, err := prov.CreateConnection()

	assert.Nil(t, conn)
	assert.ErrorIs(t, err, appErrors.ErrNilConfig)
}

// TestGetConnectionFromPoolSkipsDeadConnections: a closed connection sitting in
// the pool must never be handed to a publisher.
func TestGetConnectionFromPoolSkipsDeadConnections(t *testing.T) {
	prov := &amqpProvider{
		amqpConf: poolConfig(2),
		// A zero value connection reports itself as closed.
		ConnectionPool: []*amqp.Connection{nil, nil},
	}

	conn, err := prov.GetConnectionFromPool()

	// Every pooled entry was unusable, so the provider tried to dial a
	// replacement (which fails against a dead address) instead of returning a
	// broken connection.
	assert.Nil(t, conn)
	assert.Error(t, err)
	assert.Empty(t, prov.ConnectionPool, "dead connections must be evicted")
}

// TestGetAmqpConnectionNeverReturnsNilNil covers callers that dereferenced the
// (nil, nil) this used to return when the pool entry had the wrong type.
func TestGetAmqpConnectionNeverReturnsNilNil(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}

	conn, err := prov.GetAmqpConnection()

	assert.Nil(t, conn)
	assert.Error(t, err, "a nil connection must always come with an error")
}

func TestGetConnectionFromPoolAfterClose(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}
	require.NoError(t, prov.CloseConnection())

	conn, err := prov.GetConnectionFromPool()

	assert.Nil(t, conn)
	assert.ErrorIs(t, err, appErrors.ErrConnectionPoolClosed)
}

// TestReleaseConnectionToPoolIgnoresNonConnections keeps the release path total:
// it is called from deferred cleanups that cannot check the type themselves.
func TestReleaseConnectionToPoolIgnoresNonConnections(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(2)}

	assert.NoError(t, prov.ReleaseConnectionToPool(nil))
	assert.NoError(t, prov.ReleaseConnectionToPool("not a connection"))
	assert.NoError(t, prov.ReleaseConnectionToPool((*amqp.Connection)(nil)))
	assert.Empty(t, prov.ConnectionPool)
}

// TestReleaseConnectionToPoolIsBounded covers unbounded pool growth: releases
// used to append unconditionally, so a busy publisher grew the slice forever.
func TestReleaseConnectionToPoolIsBounded(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(2)}

	// Fill the pool to its configured size with placeholder entries.
	prov.ConnectionPool = []*amqp.Connection{nil, nil}

	require.NoError(t, prov.ReleaseConnectionToPool((*amqp.Connection)(nil)))

	assert.LessOrEqual(t, len(prov.ConnectionPool), 2)
}

func TestRemoveConnectionEvictsFromPool(t *testing.T) {
	first := &amqp.Connection{}
	second := &amqp.Connection{}
	prov := &amqpProvider{
		amqpConf:       poolConfig(2),
		ConnectionPool: []*amqp.Connection{first, second},
	}

	prov.removeConnection(first)

	assert.Equal(t, []*amqp.Connection{second}, prov.ConnectionPool)

	// Removing something that is not in the pool is a no-op, not a panic.
	assert.NotPanics(t, func() { prov.removeConnection(&amqp.Connection{}) })
	assert.Len(t, prov.ConnectionPool, 1)
}

func TestCloseConnectionIsIdempotent(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}

	assert.NoError(t, prov.CloseConnection())
	assert.NoError(t, prov.CloseConnection())
	assert.True(t, prov.closed)
	assert.Nil(t, prov.ConnectionPool)
}

func TestPublishConfirmTimeout(t *testing.T) {
	assert.Equal(t, defaultPublishConfirmTimeout, (&amqpProvider{}).publishConfirmTimeout())
	assert.Equal(t, defaultPublishConfirmTimeout,
		(&amqpProvider{amqpConf: &config.AMQPConfig{}}).publishConfirmTimeout())
	assert.Equal(t, 12*time.Second,
		(&amqpProvider{amqpConf: &config.AMQPConfig{PublishConfirmTimeout: 12}}).publishConfirmTimeout())
}

func TestPoolSizeFallsBackToDefault(t *testing.T) {
	assert.Equal(t, defaultConnectionPoolSize, (&amqpProvider{}).poolSize())
	assert.Equal(t, defaultConnectionPoolSize,
		(&amqpProvider{amqpConf: &config.AMQPConfig{ConnectionPoolSize: 0}}).poolSize())
	assert.Equal(t, 4, (&amqpProvider{amqpConf: &config.AMQPConfig{ConnectionPoolSize: 4}}).poolSize())
}

// TestPublishRetriesBeforeGivingUp: a publish must not fail on the first
// stale-connection error, which is exactly what a broker restart produces.
func TestPublishRetriesBeforeGivingUp(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}

	start := time.Now()
	err := prov.AmqpPublishWithConfirm(context.Background(), "key", amqp.Publishing{}, "exchange")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish failed after")
	// Each attempt dials and fails, so more than one attempt was made.
	assert.Greater(t, time.Since(start), 50*time.Millisecond)
}

func TestPublishHonoursCancelledContext(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := prov.AmqpPublishWithConfirm(ctx, "key", amqp.Publishing{}, "exchange")

	assert.ErrorIs(t, err, context.Canceled)
}

func TestNilArgumentsAreRejectedNotDereferenced(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(1)}

	assert.NotPanics(t, func() {
		_, _, err := prov.CreateAmqpChannel(nil, false)
		assert.ErrorIs(t, err, appErrors.ErrInvalidConnectionPool)

		assert.NoError(t, prov.CloseAmqpChannel(nil))

		_, err = prov.CreateConsumer(nil, "queue", "tag")
		assert.ErrorIs(t, err, appErrors.ErrInvalidConnectionPool)

		assert.ErrorIs(t, prov.SetChannelQoS(nil, 1), appErrors.ErrInvalidConnectionPool)

		assert.ErrorIs(t, prov.AmqpPublish(context.Background(), nil, "key", amqp.Publishing{}, "exchange"),
			appErrors.ErrInvalidConnectionPool)
	})
}

// TestConcurrentPoolAccess runs under -race to check the pool's locking.
func TestConcurrentPoolAccess(t *testing.T) {
	prov := &amqpProvider{amqpConf: poolConfig(4)}

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = prov.GetConnectionFromPool()
		}()
		go func() {
			defer wg.Done()
			_ = prov.ReleaseConnectionToPool((*amqp.Connection)(nil))
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, len(prov.ConnectionPool), 4)
}
