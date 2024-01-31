package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"sync"

	//"github.com/surendratiwari3/paota/example/task"
	"github.com/surendratiwari3/paota/internal/logger"
	"os"
)

func main() {

	logrusLog := logrus.StandardLogger()
	logrusLog.SetFormatter(&logrus.JSONFormatter{})
	logger.ApplicationLogger = logrusLog

	cnf := config.Config{
		Broker: "amqp",
		//Store:         "null",
		TaskQueueName: "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://guest:guest@localhost:55005/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
			DelayedQueue:       "delay_test",
		},
	}

	newWorkerPool, err := workerpool.NewWorkerPoolWithConfig(context.Background(), 10, "testWorker", cnf)
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		os.Exit(0)
	} else if newWorkerPool == nil {
		logger.ApplicationLogger.Info("workerPool is nil")
		os.Exit(0)
	}
	logger.ApplicationLogger.Info("newWorkerPool created successfully")
	// Register tasks
	regTasks := map[string]interface{}{
		"Print": Print,
	}
	err = newWorkerPool.RegisterTasks(regTasks)
	if err != nil {
		logger.ApplicationLogger.Info("error while registering task")
		return
	}
	logger.ApplicationLogger.Info(newWorkerPool.IsTaskRegistered("add"))

	logger.ApplicationLogger.Info("Worker is also started")

	// UserRecord represents the structure of user records.
	type UserRecord struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		// Add other fields as needed
	}

	// Replace this with the received user record
	user := UserRecord{
		ID:    "1",
		Name:  "John Doe",
		Email: "john.doe@example.com",
	}

	// Convert the struct to a JSON string
	userJSON, err := json.Marshal(user)
	if err != nil {
		//
	}

	printJob := &schema.Signature{
		Name: "Print",
		Args: []schema.Arg{
			{
				Type:  "string",
				Value: string(userJSON),
			},
		},
		RetryCount:                  10,
		IgnoreWhenTaskNotRegistered: true,
	}

	waitGrp := sync.WaitGroup{}
	waitGrp.Add(1)
	for i := 0; i < 50; i++ {
		go func() {
			for i := 0; i < 100000; i++ {
				newWorkerPool.SendTaskWithContext(context.Background(), printJob)
			}
		}()
	}

	waitGrp.Wait()
}

func Print(arg *schema.Signature) error {
	logger.ApplicationLogger.Info("Print Function In Error")
	return errors.New("checking retry")
}
