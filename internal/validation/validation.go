package validation

import (
	"github.com/surendratiwari3/paota/schema/errors"
	"reflect"
)

// ValidateTask validates task function using reflection and makes sure
// it has a proper signature. Functions used as tasks must return at least a
// single value and the last return type must be error
func ValidateTask(task interface{}) error {
	v := reflect.ValueOf(task)
	t := v.Type()

	// Task must be a function
	if t.Kind() != reflect.Func {
		return errors.ErrTaskMustBeFunc
	}

	// Task must return at least a single value
	if t.NumOut() < 1 {
		return errors.ErrTaskReturnsNoValue
	}

	// Last return value must be error
	lastReturnType := t.Out(t.NumOut() - 1)
	errorInterface := reflect.TypeOf((*error)(nil)).Elem()
	if !lastReturnType.Implements(errorInterface) {
		return errors.ErrLastReturnValueMustBeError
	}

	return nil
}
