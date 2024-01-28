package task

type TaskRegistrarInterface interface {
	IsTaskRegistered(name string) bool
	GetRegisteredTask(name string) (interface{}, error)
	RegisterTasks(namedTaskFuncs map[string]interface{}) error
	GetRegisteredTaskCount() uint
}
