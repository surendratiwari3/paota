package task

import (
	"bytes"
	"context"
	"encoding/json"
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
