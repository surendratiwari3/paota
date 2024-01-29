package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/surendratiwari3/paota/errors"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"io/ioutil"
	"net/http"
)

// AddTaskHandlerV1 handles the request to add a task for API version 1
func AddTaskHandlerV1(c echo.Context) error {
	// Parse request body to get the Signature
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to read request body")
	}

	taskName := c.Param("task_name")

	var signature schema.Signature
	if err := json.Unmarshal(body, &signature); err != nil {
		return c.String(http.StatusBadRequest, "Invalid JSON payload")
	}
	signature.Name = taskName
	signature.UUID = uuid.NewString()
	validate := validator.New(validator.WithRequiredStructEnabled())

	// Validate the Signature
	if err := validate.Struct(signature); err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid Signature: %s", err.Error()))
	}

	// Add the task based on the received Signature and task_name
	err = EnqueueTask(&signature)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to add task: %s", err.Error()))
	}

	return c.String(http.StatusOK, "Task added successfully")
}

func EnqueueTask(job *schema.Signature) error {
	newWorkerPool, err := workerpool.NewWorkerPool(context.Background(), 10, "testWorker")
	if err != nil {
		logger.ApplicationLogger.Error("workerPool is not created", err)
		return err
	} else if newWorkerPool == nil {
		logger.ApplicationLogger.Info("workerPool is nil")
		return errors.ErrInvalidConfig
	}
	_, err = newWorkerPool.SendTaskWithContext(context.Background(), job)
	return err
}
