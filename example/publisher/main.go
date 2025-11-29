package main

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"os"
	"sync"
)

type UserRecord struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {

	// Setup logger
	logrusLog := logrus.StandardLogger()
	logrusLog.SetFormatter(&logrus.JSONFormatter{})
	logger.ApplicationLogger = logrusLog

	// Config
	cnf := config.Config{
		Broker:        "amqp",
		TaskQueueName: "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
			DelayedQueue:       "delay_test",
		},
	}

	// Create worker pool
	newWorkerPool, err := workerpool.NewWorkerPoolWithConfig(context.Background(), 10, "testWorker", cnf)
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		os.Exit(1)
	}
	logger.ApplicationLogger.Info("workerPool created successfully")

	// Build message payload
	user := UserRecord{ID: "1", Name: "John Doe", Email: "john.doe@example.com"}
	userJSON, _ := json.Marshal(user)

	printJob := &schema.Signature{
		Name:                        "Print",
		RawArgs:                     userJSON,
		TaskTimeOut:                 100,
		RetryCount:                  10,
		IgnoreWhenTaskNotRegistered: true,
	}

	// TOTAL messages to publish
	totalMessages :=
		(50 * 50000) + // 50 goroutines × 50k messages
			(10 * 50000) // 10 goroutines × 50k messages

	logger.ApplicationLogger.Infof("Publishing %d messages ...", totalMessages)

	var wg sync.WaitGroup
	wg.Add(totalMessages)

	// First batch: 50 goroutines × 50k tasks
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 50000; j++ {
				newWorkerPool.SendTaskWithContext(context.Background(), printJob)
				wg.Done()
			}
		}()
	}

	// Second batch: 10 goroutines × 50k tasks
	printJob.TaskTimeOut = 120
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 50000; j++ {
				newWorkerPool.SendTaskWithContext(context.Background(), printJob)
				wg.Done()
			}
		}()
	}

	// Wait for all messages to be published
	wg.Wait()

	logger.ApplicationLogger.Infof("All %d messages published successfully!", totalMessages)
}
