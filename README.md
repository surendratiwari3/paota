# surendratiwari3/paota [![CircleCI](https://dl.circleci.com/status-badge/img/gh/surendratiwari3/paota/tree/main.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/surendratiwari3/paota/tree/main)

An efficient Go task queue package, facilitating the seamless orchestration and execution of tasks. Alternative to machinery and Celery.

## Architecture
![Architecture Diagram](https://github.com/surendratiwari3/paota/blob/main/docs/images/paota_arch.png)

## paota To-Do List

### In Progress
- [ ] Update documentation for new features.
- [ ] Unit test and code coverage
- [ ] Middleware for task
- [ ] Error Callback for task
- [ ] Logging format with taskId

### Planned
- [ ] UI/UX for better engagement.
- [ ] Scheduled task
- [ ] Release first version 1.0.0.

### Future
- [ ] Integrate third-party API for additional functionality.
- [ ] Conduct a security audit.
- [ ] Multi Queue
- [ ] Ratelimit based consuming
- [ ] Consume over Webhook
- [ ] API for task management (create/delete/update/get/list)
- [ ] Webhook for task events
- [ ] CI/CD integration to provide the docker image over dockerhub
- [ ] Standard tasks based on ongoing challenges across industry
- [ ] SDK for PHP/NodeJS
- [ ] APM hook for newrelic
- [ ] Custom Job Signature

### Completed
- [x] Initial project setup.
- [x] AMQP Connection Pool
- [x] Publish Task
- [x] Logger Interface
- [x] WorkerPool Supported
- [x] Consumer with concurrency added
- [x] Consumer task processor based on defined task added
- [x] CircleCI Integration with generating mock and running unit test 

## Features

- **User-Defined Tasks:** Users can define their own tasks.
- **Message Broker:** Utilizes RabbitMQ for task queuing.
- **Backend Storage:** for storing and updating task information. (Optional)

## Getting Started

### Prerequisites

- Go (version 1.21 or higher)
- RabbitMQ (installation guide: https://www.rabbitmq.com/download.html)

### Configuration

The `Config` struct holds all configuration options for Paota. It includes the following parameters:

- **Broker**: The message broker to be used. Currently, only "amqp" is supported.
- **Store**: The type of storage to be used (optional).
- **TaskQueueName**: The name of the task queue. Default value is "paota_tasks".
- **StoreQueueName**: The name of the storage queue (optional).
- **AMQP**: Configuration for the AMQP (RabbitMQ) connection. See [AMQP Configuration](#amqp-configuration) for details.
- **MongoDB**: Configuration for MongoDB integration. See [MongoDB Configuration](#mongodb-configuration) for details.

Here's an example of how you can set up the main configuration using environment variables:

```bash
export BROKER=amqp
export STORE=mongodb
export QUEUE_NAME=paota_tasks
export STORE_QUEUE_NAME=your_store_queue_name
```

#### AMQP Configuration

Before using the `paota` package, it's essential to understand the AMQP (Advanced Message Queuing Protocol) configuration used by the package. The configuration is encapsulated in the `AMQPConfig` struct, which includes the following parameters:

- **Url**: The connection URL for RabbitMQ. It is a required field.
- **Exchange**: The name of the RabbitMQ exchange to be used for task queuing. Default value is "paota_task_exchange".
- **ExchangeType**: The type of the RabbitMQ exchange. Default value is "direct".
- **QueueDeclareArgs**: Arguments used when declaring a queue. It is of type `QueueDeclareArgs`, which is a map of string to interface{}.
- **QueueBindingArgs**: Arguments used when binding to the exchange. It is of type `QueueBindingArgs`, which is a map of string to interface{}.
- **BindingKey**: The routing key used for binding to the exchange. Default value is "paota_task_binding_key".
- **PrefetchCount**: The number of messages to prefetch from the RabbitMQ server. It is a required field.
- **AutoDelete**: A boolean flag indicating whether the queue should be automatically deleted when there are no consumers. Default is false.
- **DelayedQueue**: The name of the delayed queue for handling delayed tasks. Default value is "paota_task_delayed_queue".
- **ConnectionPoolSize**: The size of the connection pool for RabbitMQ connections. Default value is 5.
- **HeartBeatInterval**: The interval (in seconds) at which heartbeats are sent to RabbitMQ. Default value is 10.

Here's an example of how you can configure the `AMQPConfig` using environment variables:

```bash
export URL=amqp://guest:guest@localhost:5672/
export EXCHANGE=paota_task_exchange
export EXCHANGE_TYPE=direct
export BINDING_KEY=paota_task_binding_key
export PREFETCH_COUNT=5
export AUTO_DELETE=false
export DELAYED_QUEUE=paota_task_delayed_queue
export CONNECTION_POOL_SIZE=5
export HEARTBEAT_INTERVAL=10
```

#### MongoDB Configuration

For MongoDB integration, the `MongoDBConfig` struct is used:

```go
type MongoDBConfig struct {
Client   *mongo.Client
Database string
}
```

### Signature Structure

The `Signature` struct represents a single task invocation and has the following fields:

- `UUID`: Unique identifier for the task.
- `Name`: Name of the task.
- `Args`: List of arguments for the task, where each `Arg` has the following structure:
    - `Name`: Name of the argument.
    - `Value`: Value of the argument.
- `RoutingKey`: Routing key for the task.
- `Priority`: Priority of the task.
- `RetryCount`: Number of times the task can be retried.
- `RetryTimeout`: Timeout duration for retrying the task.
- `IgnoreWhenTaskNotRegistered`: Flag to indicate whether to ignore the task when not registered.

### Task Function Format
- **User-Defined Tasks:**
```bash
- function_name(arg *task.Signature) error
```

### Publisher (Enqueue Task)
In order to enqueue jobs, you'll need to make a WorkerPool. publish jobs to broker.
```go
package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/workerpool"
)

func main() {
	// Set up the logger
	logger.ApplicationLogger = logrus.StandardLogger()

	// Configure Paota
	cnf := config.Config{
		Broker:         "amqp",
		TaskQueueName:  "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://guest:guest@localhost:55005/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
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
	logger.ApplicationLogger.Info(newWorkerPool.IsTaskRegistered("Print"))

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

	printJob := &task.Signature{
		Name: "Print",
		Args: []task.Arg{
			{
				Type:  "string",
				Value: string(userJSON),
			},
		},
		IgnoreWhenTaskNotRegistered: true,
	}

	go func() {
		for i := 0; i < 100000; i++ {
			newWorkerPool.SendTaskWithContext(context.Background(), printJob)
		}
	}()
}

func Print(arg *task.Signature) error {
	logger.ApplicationLogger.Info("Print Function Completed")
	return nil
}
```

### Consumer (Task Processor)
In order to process jobs, you'll need to make a WorkerPool. Add jobs to the pool, and start the pool.
```go
package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/task"
	"github.com/surendratiwari3/paota/workerpool"
)

func main() {
	// Set up the logger
	logger.ApplicationLogger = logrus.StandardLogger()

	// Configure Paota
	cnf := config.Config{
		Broker:         "amqp",
		TaskQueueName:  "paota_task_queue",
		AMQP: &config.AMQPConfig{
			Url:                "amqp://guest:guest@localhost:55005/",
			Exchange:           "paota_task_exchange",
			ExchangeType:       "direct",
			BindingKey:         "paota_task_binding_key",
			PrefetchCount:      100,
			ConnectionPoolSize: 10,
		},
	}

	err := config.GetConfigProvider().SetApplicationConfig(cnf)
	if err != nil {
		logger.ApplicationLogger.Error("config error", err)
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
	logger.ApplicationLogger.Info(newWorkerPool.IsTaskRegistered("Print"))

	logger.ApplicationLogger.Info("Worker is also started")

	// Start the worker pool
	newWorkerPool.Start()
}

// Print is an example task function
func Print(arg *task.Signature) error {
	logger.ApplicationLogger.Info("Print Function Completed")
	return nil
}

```


### Mocks for this repository are generated using mockery(v2)
```bash
mockery --all --output=mocks
```

## Benchmarks

### Benchmark Configuration
Information about parameters and configurations for each benchmark will be added in the future.

### Running the Benchmarks
Instructions on how to run the benchmarks will be provided once benchmarks are available.

### Interpreting Results
Guidance on how to interpret benchmark results and metrics will be added.

### Comparison and Analysis (Future)
Plans for adding new benchmarks and criteria for doing so will be discussed in this section.

### Conclusion
A summary of the importance of benchmarking and plans for future benchmarking activities will be provided.


