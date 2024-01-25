package main

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/example/task"
	"github.com/surendratiwari3/paota/logger"
	paotaTask "github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/workerpool"
	"os"
)

func main() {
	cnf := &config.Config{
		Broker:        "amqp",
		Store:         "null",
		TaskQueueName: "machine",
		AMQP: &config.AMQPConfig{
			Url:          "amqp://guest:guest@localhost:55005/",
			Exchange:     "machinery_exchange",
			ExchangeType: "direct",
			BindingKey:   "machinery_tasks_1",
			//PrefetchCount: 100,
		},
	}
	logger.ApplicationLogger = logrus.StandardLogger()
	newWorkerPool, err := workerpool.NewWorkerPool(cnf, 10, "testWorker")
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
		"add": task.Add,
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

	store2Mongo := &paotaTask.Signature{
		Name: "WriteToMongoTask",
		Args: []paotaTask.Arg{
			{
				Type:  "string",
				Value: string(userJSON),
			},
		},
		IgnoreWhenTaskNotRegistered: true,
	}
	newWorkerPool.SendTaskWithContext(context.Background(), store2Mongo)
}
