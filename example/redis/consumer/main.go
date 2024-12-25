package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
)

type printWorker struct {
	workerPool workerpool.Pool
}

type retryTestWorker struct {
    workerPool workerpool.Pool
    attempts   map[string]int  // Track attempts per task
    mu         sync.Mutex      // Protect the map
}

type scheduledWorker struct {
    workerPool workerpool.Pool
}

func main() {
	// Configure Redis Broker
	cnf := config.Config{
		Broker:        "redis",
		TaskQueueName: "paota_task_queue",
		Redis: &config.RedisConfig{
			Address: "localhost:6379", // Replace with your Redis server address
			DelayedTasksKey: "paota_delayed_tasks", // Key for delayed/scheduled tasks
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

	// Create the worker instances
	printWorker := printWorker{workerPool: newWorkerPool}
	retryWorker := &retryTestWorker{
        workerPool: newWorkerPool,
        attempts: make(map[string]int),
    }
	scheduledWorker := scheduledWorker{workerPool: newWorkerPool}

	logger.ApplicationLogger.Info("newWorkerPool created successfully")

	// Register tasks
	regTasks := map[string]interface{}{
		"Print": printWorker.Print,
		"RetryTest": retryWorker.RetryTest,
		"ScheduledTask": scheduledWorker.ScheduledTask,
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

func (w *retryTestWorker) RetryTest(arg *schema.Signature) error {
    w.mu.Lock()
    w.attempts[arg.UUID]++
    attempts := w.attempts[arg.UUID]
    w.mu.Unlock()

    logger.ApplicationLogger.Info("Processing RetryTest task", 
        "taskID", arg.UUID,
        "attempt", attempts,
        "data", arg.Args[0].Value,
    )

    // Fail first 3 attempts
    if attempts <= 3 {
        return fmt.Errorf("intentional failure, attempt %d/3", attempts)
    }

    // Succeed on 4th attempt
    logger.ApplicationLogger.Info("RetryTest task succeeded", 
        "taskID", arg.UUID,
        "attempts", attempts,
    )
    return nil
}

func (sw scheduledWorker) ScheduledTask(arg *schema.Signature) error {
    // Log when the task actually executes
    executionTime := time.Now().UTC()
    
    // Parse the scheduled time from args if provided
    var scheduledTime time.Time
    if len(arg.Args) > 0 {
        if timeStr, ok := arg.Args[0].Value.(string); ok {
            if parsed, err := time.Parse(time.RFC3339, timeStr); err == nil {
                scheduledTime = parsed
            }
        }
    }

    logger.ApplicationLogger.Info("Executing scheduled task",
        "taskID", arg.UUID,
        "scheduledFor", scheduledTime,
        "executedAt", executionTime,
        "data", arg.Args,
    )

    // Calculate drift if we have a scheduled time
    if !scheduledTime.IsZero() {
        drift := executionTime.Sub(scheduledTime)
        logger.ApplicationLogger.Info("Scheduled task timing",
            "taskID", arg.UUID,
            "driftSeconds", drift.Seconds(),
        )
    }

    return nil
}
