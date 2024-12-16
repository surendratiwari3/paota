package config

type RedisConfig struct {
	Address string `yaml:"redis_Address" envconfig:"REDIS_ADDRESS" envDefault:"redis://localhost:6379"`
	// Maximum number of idle connections in the pool.
	MaxIdle int `yaml:"max_idle" envconfig:"REDIS_MAX_IDLE"`

	// Maximum number of connections allocated by the pool at a given time.
	// When zero, there is no limit on the number of connections in the pool.
	// Default: 100
	MaxActive int `yaml:"max_active" envconfig:"REDIS_MAX_ACTIVE"`

	// Close connections after remaining idle for this duration in seconds. If the value
	// is zero, then idle connections are not closed. Applications should set
	// the timeout to a value less than the server's timeout.
	// Default: 300
	IdleTimeout int `yaml:"max_idle_timeout" envconfig:"REDIS_IDLE_TIMEOUT"`

	// If Wait is true and the pool is at the MaxActive limit, then Get() waits
	// for a connection to be returned to the pool before returning.
	// Default: true
	Wait bool `yaml:"wait" envconfig:"REDIS_WAIT"`

	// ReadTimeout specifies the timeout in seconds for reading a single command reply.
	// Default: 15
	ReadTimeout int `yaml:"read_timeout" envconfig:"REDIS_READ_TIMEOUT"`

	// WriteTimeout specifies the timeout in seconds for writing a single command.
	// Default: 15
	WriteTimeout int `yaml:"write_timeout" envconfig:"REDIS_WRITE_TIMEOUT"`

	// ConnectTimeout specifies the timeout in seconds for connecting to the Redis server when
	// no DialNetDial option is specified.
	// Default: 15
	ConnectTimeout int `yaml:"connect_timeout" envconfig:"REDIS_CONNECT_TIMEOUT"`

	// NormalTasksPollPeriod specifies the period in milliseconds when polling redis for normal tasks
	// Default: 1000
	NormalTasksPollPeriod int `yaml:"normal_tasks_poll_period" envconfig:"REDIS_NORMAL_TASKS_POLL_PERIOD"`

	// DelayedTasksPollPeriod specifies the period in milliseconds when polling redis for delayed tasks
	// Default: 20
	DelayedTasksPollPeriod int    `yaml:"delayed_tasks_poll_period" envconfig:"REDIS_DELAYED_TASKS_POLL_PERIOD"`
	DelayedTasksKey        string `yaml:"delayed_tasks_key" envconfig:"REDIS_DELAYED_TASKS_KEY"`

	// ClientName specifies the redis client name to be set when connecting to the Redis server
	ClientName string `yaml:"client_name" envconfig:"REDIS_CLIENT_NAME"`

	// MasterName specifies a redis master name in order to configure a sentinel-backed redis FailoverClient
	MasterName string `yaml:"master_name" envconfig:"REDIS_MASTER_NAME"`

	// ClusterEnabled specifies whether cluster mode is enabled, regardless the number of addresses.
	// This helps create ClusterClient for Redis servers that enabled cluster mode with 1 node, or using AWS configuration endpoint
	ClusterEnabled bool `yaml:"cluster_enabled" envconfig:"REDIS_CLUSTER_ENABLED"`
}