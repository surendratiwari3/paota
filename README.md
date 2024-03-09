# paota [![CircleCI](https://dl.circleci.com/status-badge/img/gh/surendratiwari3/paota/tree/main.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/surendratiwari3/paota/tree/main)

[![Quality gate](https://sonarcloud.io/api/project_badges/quality_gate?project=surendratiwari3_paota)](https://sonarcloud.io/summary/new_code?id=surendratiwari3_paota)

An efficient Go task queue package, facilitating the seamless orchestration and execution of tasks. Alternative to machinery and Celery.

## Architecture
![Architecture Diagram](https://github.com/surendratiwari3/paota/blob/main/docs/images/TopDownPaota.png?raw=true)

## Overview

Below is a high-level overview of how Paota works:

### 1. Publisher

The process or component responsible for generating tasks and putting them onto the task queue. This could be any part of your system that needs to perform background or asynchronous work.

### 2. Task Queue

Paota uses a task queue as a central mechanism for managing and distributing tasks. The task queue holds the tasks until they are picked up by consumers for processing. The queue serves as a buffer, decoupling the production of tasks from their consumption.

### 3. Broker

The broker is a service or component responsible for managing the task queue. It receives tasks from the publisher and makes them available for consumption by consumers. The broker ensures that tasks are delivered reliably and efficiently to the consumers.

### 4. Consumer

The consumer is a process or component that pulls tasks from the task queue and executes them. Paota allows for multiple consumers to run concurrently, enabling parallel processing of tasks. Consumers can be distributed across multiple machines for horizontal scaling.

### 5. Worker Pool

Each consumer runs a worker pool, which is a group of worker processes that execute tasks concurrently. The worker pool allows for efficient utilization of resources by processing multiple tasks simultaneously. The number of workers in a pool can be adjusted based on the available resources and workload.

### 6. Task Processing

Tasks are processed by the worker pool concurrently. Each task represents a unit of work that needs to be performed asynchronously. The results of the task execution can be used to update the state of the system or trigger additional actions.

### 7. Storage

Paota may use storage to persist task-related information, ensuring durability and fault tolerance. This can include storing task state, metadata, and other relevant information. The choice of storage can vary, and Paota supports different storage backends.

### 8. High Availability and Horizontal Scaling

Paota is designed to provide high availability and support horizontal scaling. By distributing tasks across multiple consumers and worker pools, the system can handle increased workloads and provide fault tolerance. Additionally, the use of multiple brokers and storage solutions contributes to the overall resilience of the system.

In summary, Paota facilitates the asynchronous processing of tasks in a distributed environment, allowing for efficient utilization of resources, high availability, and horizontal scaling. It is a versatile tool for building scalable and responsive systems that can handle background and asynchronous workloads.

## Quest for Completion

### In Progress
- [ ] Middleware for task
- [ ] Logging format
- [ ] Middleware Support for WorkerPool (requestId in log, authentication, logging)
- [ ] MongoBackendStorage
- [ ] Pre and Post Hook at Task Level
- [ ] Workflow support
- [ ] Chaining of Task support
- [ ] Coverage Check integration with sonarqube

### Planned
- [ ] API for task management (create/delete/update/get/list)
- [ ] Release first version 1.0.0.

### Future
- [ ] Integrate third-party API for additional functionality.
- [ ] Conduct a security audit.
- [ ] UI/UX for better engagement.
- [ ] Multi Queue
- [ ] Ratelimit based consuming
- [ ] Consume over Webhook
- [ ] Webhook for task events
- [ ] CI/CD integration to provide the docker image over dockerhub
- [ ] Standard tasks based on ongoing challenges across industry
- [ ] SDK for PHP/NodeJS
- [ ] APM hook for newrelic
- [ ] Custom Job Signature
- [ ] Cloud based serverless task execution using api

### Completed
- [x] Initial project setup.
- [x] AMQP Connection Pool
- [x] Publish Task
- [x] Logger Interface
- [x] WorkerPool Supported
- [x] Consumer with concurrency added
- [x] Consumer task processor based on defined task added
- [x] CircleCI Integration with generating mock and running unit test
- [x] Unit test and code coverage
- [x] Retry for task for amqp broker
- [x] SAST check integration with sonarcloud and codeql
- [x] Schedule Task

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
export URL=amqp://guest:guest@localhost:55005/
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
example/producer/main.go
```

### Consumer (Task Processor)
In order to process jobs, you'll need to make a WorkerPool. Add jobs to the pool, and start the pool.

```go
example/consumer/main.go
```

### Mocks for this repository are generated using mockery(v2)
```bash
mockery --inpackage --all
```

## Benchmarks

### Benchmark Configuration

All these benchmarks are done in a notebook with these configuration:

```bash
Processor: 1.8 GHz Dual-Core Intel Core i5
Memory: 8 GB 1600 MHz DDR3

```
The jobs are almost no-op jobs: they simply return nil. Rabbitmq , Consumer and Publisher running on same server

### RabbitMQ to MongoDB Data Processing Performance Test

This repository contains a performance test script for consuming data from RabbitMQ and inserting it into MongoDB with a uniform structure. The goal of this test is to ensure that the system can handle a high message throughput rate while consuming and processing more than 10 lakh (10 million) data records within 5 minutes, with an acknowledgment rate of more than 12k messages per second and same data stored in MongoDb. The test is conducted on a MacBook with 16GB RAM, and both MongoDB and RabbitMQ are hosted locally.

![Ack Rate](https://github.com/manishjha1991/paota/blob/performance-mongodb-ack-rate/docs/images/ackrate.png?raw=true)




![MonGoDb](https://github.com/manishjha1991/paota/blob/performance-mongodb-ack-rate/docs/images/mongodbRecord.png?raw=true)

### Conclusion
We have acheived benchmarking for 50 publisher publishing request and 1 consumer worker consuming the request at speed of 7000 request per second (concurrency=10 and PrefetchCount=100). If you want to achieve more throughput concurrency can be increased to any extent. 

Thank you for flying Paota!


