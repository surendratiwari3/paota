package redis

import (
	"context"

	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/provider"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/schema"
)

type RedisBroker struct {
	provider provider.RedisProviderInterface
	queue    string
}

func NewRedisBroker(configProvider config.ConfigProvider) (broker.Broker, error) {
	redisConfig := configProvider.GetConfig().Redis
	provider := provider.NewRedisProvider(redisConfig)
	return &RedisBroker{
		provider: provider,
		queue:    configProvider.GetConfig().TaskQueueName,
	}, nil
}

func (rb *RedisBroker) Publish(ctx context.Context, signature *schema.Signature) error {
	return rb.provider.Publish(ctx, rb.queue, signature)
}

func (rb *RedisBroker) StartConsumer(ctx context.Context, workerGroup workergroup.WorkerGroupInterface) error {
	return rb.provider.Subscribe(rb.queue, func(signature *schema.Signature) error {
		workerGroup.AssignJob(signature)
		return nil
	})
}

func (rb *RedisBroker) StopConsumer() {
	_ = rb.provider.CloseConnection()
}

func (rb *RedisBroker) BrokerType() string {
	return "redis" // Return "redis" to indicate it's a Redis broker
}
