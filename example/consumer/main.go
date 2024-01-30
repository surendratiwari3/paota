package main

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/schema"
	//"github.com/surendratiwari3/paota/example/task"
	"github.com/surendratiwari3/paota/internal/logger"
	"github.com/surendratiwari3/paota/workerpool"
	"os"
)

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
			Url:                "amqp://guest:guest@localhost:55005/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
			DelayedQueue:       "delay_test",
		},
	}
	err := config.GetConfigProvider().SetApplicationConfig(cnf)
	if err != nil {
		logger.ApplicationLogger.Error("config error", err)
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
	logger.ApplicationLogger.Info("Worker is also started")

	err = newWorkerPool.Start()
	if err != nil {
		logger.ApplicationLogger.Error("error while starting worker")
	}
}

func Print(arg *schema.Signature) error {
	logger.ApplicationLogger.Info("Print Function Error")
	return errors.New("checking retry")
}
