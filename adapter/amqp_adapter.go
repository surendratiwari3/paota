package adapter

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/errors"
	"sync"
	"time"
)

// AMQPAdapter implements the MessagingAdapter interface for AMQP
type AMQPAdapter struct {
	amqpConfig       *config.AMQPConfig
	ConnectionPool   []*amqp.Connection
	connectionsMutex sync.Mutex
	// Add more AMQP-specific fields as needed
}

func (a *AMQPAdapter) CloseConnection() error {
	// Implement AMQP connection closing logic
	return nil
}

func NewAMQPAdapter() Adapter {
	if config.GetConfigProvider() == nil ||
		config.GetConfigProvider().GetConfig() == nil ||
		config.GetConfigProvider().GetConfig().AMQP == nil {
		return nil
	}
	amqpConfig := config.GetConfigProvider().GetConfig().AMQP
	return &AMQPAdapter{amqpConfig: amqpConfig}
}

func (a *AMQPAdapter) CreateConnection() (*amqp.Connection, error) {
	if a.amqpConfig == nil {
		return nil, errors.ErrNilConfig
	}
	conn, err := amqp.DialConfig(a.amqpConfig.Url,
		amqp.Config{
			Heartbeat: time.Duration(a.amqpConfig.HeartBeatInterval), // Adjust heartbeat interval as needed
		},
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// CreateConnectionPool initializes the connection pool
func (a *AMQPAdapter) CreateConnectionPool() error {
	poolSize := a.amqpConfig.ConnectionPoolSize
	if poolSize < 2 {
		return errors.ErrInvalidConfig
	}
	connPool := make([]*amqp.Connection, poolSize)

	for i := 0; i < poolSize; i++ {
		conn, err := a.CreateConnection()
		if err != nil {
			return err
		}

		connPool[i] = conn
	}
	a.ConnectionPool = connPool
	return nil
}

// ReleaseConnectionToPool releases a connection back to the pool
func (a *AMQPAdapter) ReleaseConnectionToPool(conn interface{}) error {

	if amqpConnection, ok := conn.(*amqp.Connection); ok {

		a.connectionsMutex.Lock()
		defer a.connectionsMutex.Unlock()

		a.ConnectionPool = append(a.ConnectionPool, amqpConnection)
	}
	return nil
}

// GetConnectionFromPool returns a connection from the pool
func (a *AMQPAdapter) GetConnectionFromPool() (interface{}, error) {
	a.connectionsMutex.Lock()
	defer a.connectionsMutex.Unlock()

	if a.ConnectionPool == nil || len(a.ConnectionPool) == 0 {
		return nil, errors.ErrConnectionPoolEmpty
	}

	conn := a.ConnectionPool[0]
	a.ConnectionPool = a.ConnectionPool[1:]

	return conn, nil
}
