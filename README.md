# paota (WorkInProgress)
An efficient Go task queue package, facilitating the seamless orchestration and execution of tasks. Alternative to machinery and Celery.

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

### Completed
- [x] Initial project setup.
- [x] AMQP Connection Pool
- [x] Publish Task
- [x] Logger Interface
- [x] WorkerPool Supported
- [x] Consumer with concurrency added
- [x] Consumer task processor based on defined task added

## Features

- **User-Defined Tasks:** Users can define their own tasks.
- **Message Broker:** Utilizes RabbitMQ for task queuing.
- **Backend Storage:** for storing and updating task information. (Optional)

## Getting Started

### Prerequisites

- Go (version 1.21 or higher)
- RabbitMQ (installation guide: https://www.rabbitmq.com/download.html)

### Task Function Format
- **User-Defined Tasks:** function_name(arg *task.Signature) error

### Mocks for this repository are generated using mockery(v2)
mockery --all --output=mocks

