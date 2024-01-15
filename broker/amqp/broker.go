package amqp

import (
	"context"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/broker"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/task"
	"sync"
)

type Connection struct {
	QueueName    string
	connection   *amqp.Connection
	channel      *amqp.Channel
	queue        amqp.Queue
	confirmation <-chan amqp.Confirmation
	errorchan    <-chan *amqp.Error
	cleanup      chan struct{}
}

// Broker represents an AMQP broker
type Broker struct {
	Connections      map[string]*Connection
	connectionsMutex sync.RWMutex
}

func (b Broker) StartConsuming(consumerTag string, concurrency int) error {
	/*
		queueName := config.Conf.AMQP.QueueName
	*/
	return nil
}

func (b Broker) StopConsuming() error {
	//TODO implement me
	return nil
}

func (b Broker) Enqueue(ctx context.Context, task *task.Signature) error {
	//TODO implement me
	return nil
}

// New creates new Broker instance
func New(cnf *config.Config) broker.Broker {
	return &Broker{
		Connections: make(map[string]*Connection),
	}
}
