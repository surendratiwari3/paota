package null

import (
	"github.com/surendratiwari3/paota/store"
	"github.com/surendratiwari3/paota/task"
)

// Store represents a "null" store backend
type Store struct {
}

func (b Store) InsertTask(signature task.Signature) error {
	//TODO implement me
	panic("implement me")
}

func (b Store) DeleteTask(taskId string) error {
	//TODO implement me
	panic("implement me")
}

func (b Store) GetTask(taskId string) ([]task.Signature, error) {
	//TODO implement me
	panic("implement me")
}

func (b Store) GetTaskState(taskId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func NewNullBackend() store.Backend {
	return &Store{}
}
