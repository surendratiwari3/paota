package task

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"reflect"
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
	IgnoreWhenTaskNotRegistered bool
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
