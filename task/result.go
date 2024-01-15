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
