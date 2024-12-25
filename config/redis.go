package config

type RedisConfig struct {
	Address                string `env:"URL" envDefault:"redis://localhost:6379"`
	MaxIdle                int    `env:"MAX_IDLE" envDefault:"10"`
	MaxActive              int    `env:"MAX_ACTIVE" envDefault:"100"`
	IdleTimeout            int    `env:"IDLE_TIMEOUT" envDefault:"300"`
	Wait                   bool   `env:"WAIT" envDefault:"true"`
	ReadTimeout            int    `env:"READ_TIMEOUT" envDefault:"15"`
	WriteTimeout           int    `env:"WRITE_TIMEOUT" envDefault:"15"`
	ConnectTimeout         int    `env:"CONNECT_TIMEOUT" envDefault:"15"`
	NormalTasksPollPeriod  int    `env:"NORMAL_TASKS_POLL_PERIOD" envDefault:"1000"`
	DelayedTasksPollPeriod int    `env:"DELAYED_TASKS_POLL_PERIOD" envDefault:"1000"`
	DelayedTasksKey        string `env:"DELAYED_TASKS_KEY" envDefault:"paota_tasks_delayed"`
	ClientName             string `env:"CLIENT_NAME" envDefault:"app_redis_client"`
	MasterName             string `env:"MASTER_NAME" envDefault:""`
	ClusterEnabled         bool   `env:"CLUSTER_ENABLED" envDefault:"false"`
	RetryTimeout           int    `env:"RETRY_TIMEOUT" envDefault:"30"`
	RetryCount             int    `env:"RETRY_COUNT" envDefault:"3"`
}
