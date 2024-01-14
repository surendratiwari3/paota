package null

import (
	"github.com/surendratiwari3/paota/backend"
	"github.com/surendratiwari3/paota/task"
)

// Backend represents an "null" store backend
type Backend struct {
}

func (b Backend) InsertTask(signature task.Signature) error {
	//TODO implement me
	panic("implement me")
}

func (b Backend) DeleteTask(taskId string) error {
	//TODO implement me
	panic("implement me")
}

func (b Backend) GetTask(taskId string) ([]task.Signature, error) {
	//TODO implement me
	panic("implement me")
}

func (b Backend) GetTaskState(taskId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func New() backend.Backend {
	return &Backend{}
}
