package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
)

// UserRecord represents the structure of user records.
type UserRecord struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {

	// Configure Redis broker
	cnf := config.Config{
		Broker:        "redis",
		TaskQueueName: "paota_task_queue",
		Redis: &config.RedisConfig{
			Address:         "localhost:6379",
			DelayedTasksKey: "paota_delayed_tasks",
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

	// Add this after your existing print job
	retryJob := &schema.Signature{
		Name: "RetryTest",
		Args: []schema.Arg{
			{
				Type:  "string",
				Value: "test retry mechanism",
			},
		},
		RetryCount:   5,  // Allow up to 5 retries
		RetryTimeout: 20, // Retry every 3 seconds
	}

	// Create a scheduled task to run in 1 minute
	eta := time.Now().Add(1 * time.Minute)
	scheduledJob := &schema.Signature{
		Name: "ScheduledTask",
		Args: []schema.Arg{
			{
				Type:  "string",
				Value: "This is a scheduled task",
			},
		},
		ETA: &eta,
	}

	// Send the scheduled task
	_, err = newWorkerPool.SendTaskWithContext(context.Background(), scheduledJob)
	if err != nil {
		logger.ApplicationLogger.Error("failed to send scheduled task", err)
	} else {
		logger.ApplicationLogger.Info("Scheduled task published successfully")
	}

	// Send the retry test job
	_, err = newWorkerPool.SendTaskWithContext(context.Background(), retryJob)
	if err != nil {
		logger.ApplicationLogger.Error("failed to send retry test task", err)
	} else {
		logger.ApplicationLogger.Info("Retry test task published successfully")
	}

	// Use a WaitGroup to synchronize goroutines
	var waitGrp sync.WaitGroup

	for i := 0; i < 1; i++ {
		waitGrp.Add(1) // Add to the WaitGroup counter for each goroutine
		go func() {
			defer waitGrp.Done() // Mark this goroutine as done when it exits
			for j := 0; j < 1; j++ {
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
