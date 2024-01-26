package task

import (
	"fmt"
	"github.com/surendratiwari3/paota/task"
)

// Add ...
func Add(arg *task.Signature) error {
	fmt.Println("hello world")
	return nil
}

func Print(arg *task.Signature) error {
	fmt.Println("hello world")
	return nil
}
