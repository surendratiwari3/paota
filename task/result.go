package task

// State represents an actual return value of a processed task
type State struct {
	Request  Signature
	Error    error
	Status   string
	Response Response
}

type Response struct {
	Result []interface{} `json:"result"`
}

func NewPendingTaskState(task *Signature) *State {
	TaskState := State{
		Status:  "Pending",
		Request: *task,
	}
	return &TaskState
}

func NewCompleteTaskState(task *Signature) *State {
	TaskState := State{
		Status:  "Complete",
		Request: *task,
	}
	return &TaskState
}

func NewFailedTaskState(task *Signature) *State {
	TaskState := State{
		Status:  "Failed",
		Request: *task,
	}
	return &TaskState
}

func NewCancelledTaskState(task *Signature) *State {
	TaskState := State{
		Status:  "Cancelled",
		Request: *task,
	}
	return &TaskState
}
