package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"reflect"
	"time"
)

type signatureCtxType struct{}

var signatureCtx signatureCtxType

var (
	ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()
)

// Arg represents a single argument passed to invocation fo a task
type Arg struct {
	Name  string      `bson:"name"`
	Type  string      `bson:"type"`
	Value interface{} `bson:"value"`
}

// Job wraps a signature and methods used to reflect task arguments and
// return values after invoking the task
type Job struct {
	TaskFunc   reflect.Value
	UseContext bool
	Context    context.Context
	Args       []reflect.Value
}

// Signature represents a single task invocation
type Signature struct {
	UUID                        string
	Name                        string
	Args                        []Arg
	RoutingKey                  string
	Priority                    uint8
	RetryCount                  int
	RetryTimeout                int
	RetriesDone                 int
	IgnoreWhenTaskNotRegistered bool
	ETA                         *time.Time
}

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

// NewSignature creates a new task signature
func NewSignature(name string, args []Arg) (*Signature, error) {
	signatureID := uuid.New().String()
	return &Signature{
		UUID: fmt.Sprintf("task_%v", signatureID),
		Name: name,
		Args: args,
	}, nil
}

func BytesToSignature(data []byte) (*Signature, error) {
	// Assuming that data contains a serialized JSON representation of a task.Signature
	var signature Signature

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&signature); err != nil {
		return nil, fmt.Errorf("failed to decode signature: %s", err)
	}

	return &signature, nil
}
