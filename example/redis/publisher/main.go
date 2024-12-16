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
    "sync"
)

// UserRecord represents the structure of user records.
type UserRecord struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    // Configure logger
    logrusLog := logrus.StandardLogger()
    logrusLog.SetFormatter(&logrus.JSONFormatter{})
    logger.ApplicationLogger = logrusLog

    // Configure Redis broker
    cnf := config.Config{
        Broker:        "redis",
        TaskQueueName: "paota_task_queue",
        Redis: &config.RedisConfig{
            Address: "localhost:6379",
        },
    }

    // Create a worker pool
    newWorkerPool, err := workerpool.NewWorkerPoolWithConfig(context.Background(), 10, "testWorker", cnf)
    if err != nil {
        logger.ApplicationLogger.Error("workerPool is not created", err)
        os.Exit(1)
    } else if newWorkerPool == nil {
        logger.ApplicationLogger.Info("workerPool is nil")
        os.Exit(1)
    }
    logger.ApplicationLogger.Info("newWorkerPool created successfully")

    // Prepare a user record as the task payload
    user := UserRecord{
        ID:    "1",
        Name:  "Jane Doe",
        Email: "jane.doe@example.com",
    }

    userJSON, err := json.Marshal(user)
    if err != nil {
        logger.ApplicationLogger.Error("failed to marshal user record", err)
        return
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

    // Use a WaitGroup to synchronize goroutines
    var waitGrp sync.WaitGroup

    for i := 0; i < 10; i++ {
        waitGrp.Add(1) // Add to the WaitGroup counter for each goroutine
        go func() {
            defer waitGrp.Done() // Mark this goroutine as done when it exits
            for j := 0; j < 10; j++ {
                _, err := newWorkerPool.SendTaskWithContext(context.Background(), printJob)
                if err != nil {
                    logger.ApplicationLogger.Error("failed to send task", err)
                } else {
                    logger.ApplicationLogger.Info("Task published successfully")
                }
            }
        }()
    }

    // Wait for all goroutines to finish
    waitGrp.Wait()
    logger.ApplicationLogger.Info("All tasks have been published successfully")
}
