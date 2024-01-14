# paota (WorkInProgress)
An efficient Go task queue package, facilitating the seamless orchestration and execution of tasks. Alternative to machinery and Celery.

## Features

- **User-Defined Tasks:** Users can define their own tasks.
- **Message Broker:** Utilizes RabbitMQ for task queuing.
- **Backend Storage:** Uses MongoDB as the backend for storing and updating task information. (Optional)

## Getting Started

### Prerequisites

- Go (version 1.21 or higher)
- RabbitMQ (installation guide: https://www.rabbitmq.com/download.html)
- MongoDB (installation guide: https://docs.mongodb.com/manual/installation/) - optional (if result backend in mongodb)

