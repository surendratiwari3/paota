package api

import (
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/surendratiwari3/paota/api/handlers"
	"github.com/surendratiwari3/paota/internal/config"
	"github.com/surendratiwari3/paota/internal/logger"
)

func main() {
	logrusLog := logrus.StandardLogger()
	logrusLog.SetFormatter(&logrus.JSONFormatter{})
	logger.ApplicationLogger = logrusLog

	cnf := config.Config{
		Broker: "amqp",
		//Store:         "null",
		TaskQueueName: "paota_task_queue",
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

	e := echo.New()

	v1 := e.Group("/v1/paota")

	v1.POST("/tasks/:task_name", handlers.AddTaskHandlerV1)

	// Start the server
	e.Start(":8090")
}
