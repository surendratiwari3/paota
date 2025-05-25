package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
	//"github.com/surendratiwari3/paota/example/task"
	"github.com/surendratiwari3/paota/logger"
	"os"
)

type printWorker struct {
	workerPool workerpool.Pool
}

func main() {
	logrusLog := logrus.StandardLogger()
	logrusLog.SetFormatter(&logrus.JSONFormatter{})
	logrusLog.SetReportCaller(true)

	logger.ApplicationLogger = logrusLog

	cnf := config.Config{
		Broker: "amqp",
		//Store:         "null",
		TaskQueueName: "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://localhost:5672/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
			TimeoutQueue:       "timeout_queue",
			DelayedQueue:       "delay_test",
		},
	}
	err := config.GetConfigProvider().SetApplicationConfig(cnf)
	if err != nil {
		logger.ApplicationLogger.Error("config error, exit", err)
		return
	}

	newWorkerPool, err := workerpool.NewWorkerPool(context.Background(), 10, "testWorker")
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		os.Exit(0)
	} else if newWorkerPool == nil {
		logger.ApplicationLogger.Info("workerPool is nil")
		os.Exit(0)
	}

	printWorker := printWorker{workerPool: newWorkerPool}

	logger.ApplicationLogger.Info("newWorkerPool created successfully")
	// Register tasks
	regTasks := map[string]interface{}{
		"Print": printWorker.Print,
	}
	err = newWorkerPool.RegisterTasks(regTasks)
	if err != nil {
		logger.ApplicationLogger.Info("error while registering task")
		return
	}
	logger.ApplicationLogger.Info("Worker is also started")

	err = newWorkerPool.Start()
	if err != nil {
		logger.ApplicationLogger.Error("error while starting worker")
	}
}

func (wp printWorker) Publish() {
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
		RoutingKey:                  "consumer_publisher",
		IgnoreWhenTaskNotRegistered: true,
	}

	wp.workerPool.SendTaskWithContext(context.Background(), printJob)

}

func (wp printWorker) Print(arg *schema.Signature) error {
	logger.ApplicationLogger.Info("success", arg.RawArgs)
	//wp.Publish()
	return errors.ErrUnsupported
}
