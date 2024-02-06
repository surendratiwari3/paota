package consumer

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/logger"
	"github.com/surendratiwari3/paota/internal/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"os"
	"testing"
	"time"
)

func BenchmarkAmqpNoStore(b *testing.B) {
	workerPool := SetupConsumer()
	for n := 0; n < b.N; n++ {
		Consumer(workerPool)
	}
}

func SetupConsumer() workerpool.Pool {
	logger.ApplicationLogger = logrus.StandardLogger()
	cnf := config.Config{
		Broker:        "amqp",
		TaskQueueName: "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:55005/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
		},
	}
	err := config.GetConfigProvider().SetApplicationConfig(cnf)
	if err != nil {
		logger.ApplicationLogger.Error("config error", err)
		return nil
	}
	newWorkerPool, err := workerpool.NewWorkerPool(context.Background(), 10, "testWorker")
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		os.Exit(0)
	} else if newWorkerPool == nil {
		logger.ApplicationLogger.Info("workerPool is nil")
		os.Exit(0)
	}
	logger.ApplicationLogger.Info("newWorkerPool created successfully")
	return newWorkerPool
}

func Consumer(pool workerpool.Pool) {
	// Register tasks
	regTasks := map[string]interface{}{
		"returnNil": ReturnNil,
	}
	err := pool.RegisterTasks(regTasks)
	if err != nil {
		logger.ApplicationLogger.Info("error while registering task")
		return
	}
	logger.ApplicationLogger.Info("Worker is also started")
	go func() {
		err = pool.Start()
		if err != nil {
			logger.ApplicationLogger.Error("error while starting worker")
		}
	}()
	time.Sleep(5 * time.Minute)
	pool.Stop()
}

func ReturnNil(arg *schema.Signature) error {
	return nil
}
