package consumer

import (
	"github.com/surendratiwari3/paota/bench/amqpnostorage"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"testing"
	"time"
)

func BenchmarkAmqpNoStore(b *testing.B) {
	workerPool := amqpnostorage.SetupWorkerPool()
	for n := 0; n < b.N; n++ {
		Consumer(workerPool)
	}
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
