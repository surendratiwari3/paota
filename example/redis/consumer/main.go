package main

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"github.com/surendratiwari3/paota/logger"
	"os"
)

type printWorker struct {
	workerPool workerpool.Pool
}

func main() {
	// Configure logger
	logrusLog := logrus.StandardLogger()
	logrusLog.SetFormatter(&logrus.JSONFormatter{})
	logrusLog.SetReportCaller(true)
	logger.ApplicationLogger = logrusLog

	// Configure Redis Broker
	cnf := config.Config{
		Broker: "redis",
		TaskQueueName: "paota_task_queue",
		Redis: &config.RedisConfig{
			Address:  "localhost:6379", // Replace with your Redis server address
		},
	}

	// Set the configuration
	err := config.GetConfigProvider().SetApplicationConfig(cnf)
	if err != nil {
		logger.ApplicationLogger.Error("config error, exit", err)
		return
	}

	// Create a new worker pool
	newWorkerPool, err := workerpool.NewWorkerPool(context.Background(), 10, "testWorker")
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		os.Exit(0)
	} else if newWorkerPool == nil {
		logger.ApplicationLogger.Info("workerPool is nil")
		os.Exit(0)
	}

	// Create the worker instance
	printWorker := printWorker{workerPool: newWorkerPool}

	logger.ApplicationLogger.Info("newWorkerPool created successfully")

	// Register tasks
	regTasks := map[string]interface{}{
		"Print": printWorker.Print,
	}
	err = newWorkerPool.RegisterTasks(regTasks)
	if err != nil {
		logger.ApplicationLogger.Error("error while registering tasks", err)
		return
	}

	logger.ApplicationLogger.Info("Worker is also started")

	// Start the worker pool
	err = newWorkerPool.Start()
	if err != nil {
		logger.ApplicationLogger.Error("error while starting worker", err)
	}
}

// Print is the task handler for the "Print" task
func (wp printWorker) Print(arg *schema.Signature) error {
	// Deserialize the task argument
	var user map[string]interface{}
	err := json.Unmarshal([]byte(arg.Args[0].Value.(string)), &user)
	if err != nil {
		logger.ApplicationLogger.Error("failed to parse task argument", err)
		return err
	}

	logger.ApplicationLogger.Infof("Processing task: %v", user)
	return nil
}
