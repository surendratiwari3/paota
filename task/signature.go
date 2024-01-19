package task

import (
	"fmt"
	"github.com/google/uuid"
)

// Arg represents a single argument passed to invocation fo a task
type Arg struct {
	Name  string      `bson:"name"`
	Type  string      `bson:"type"`
	Value interface{} `bson:"value"`
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
