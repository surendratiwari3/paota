package errors

import (
	"fmt"
	"time"
)

type RetryError struct {
	RetryCount int
	RetryAfter time.Duration
	Err        error
}

func (re *RetryError) Error() string {
	return fmt.Sprintf("Retry failed after %d retries. Retry after %s. Original error: %v", re.RetryCount, re.RetryAfter, re.Err)
}
