package adapter

// Adapter defines methods for a messaging system adapter
type Adapter interface {
	CloseConnection() error
	CreateConnectionPool() error
	ReleaseConnectionToPool(interface{}) error
	GetConnectionFromPool() (interface{}, error)
	// Add more methods as needed for messaging system interaction
}
