# paota (WorkInProgress)
An efficient Go task queue package, facilitating the seamless orchestration and execution of tasks. Alternative to machinery and Celery.

## paota To-Do List

### In Progress
- [ ] Update documentation for new features.
- [ ] Publish to amqp broker
- [ ] Consumer from amqp broker

### Planned
- [ ] UI/UX for better engagement.
- [ ] Add unit tests.
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

## Features

- **User-Defined Tasks:** Users can define their own tasks.
- **Message Broker:** Utilizes RabbitMQ for task queuing.
- **Backend Storage:** Uses MongoDB as the backend for storing and updating task information. (Optional)

## Getting Started

### Prerequisites

- Go (version 1.21 or higher)
- RabbitMQ (installation guide: https://www.rabbitmq.com/download.html)
- MongoDB (installation guide: https://docs.mongodb.com/manual/installation/) - optional (if result backend in mongodb)

