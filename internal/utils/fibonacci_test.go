package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFibonacci(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"negative input", -5, 0},
		{"zero", 0, 0},
		{"one", 1, 1},
		{"two", 2, 1},
		{"three", 3, 2},
		{"four", 4, 3},
		{"five", 5, 5},
		{"ten", 10, 55},
		{"twenty", 20, 6765},
		{"largest exact value", 92, 7540113804746346429},
		// Beyond the 64-bit range the result is clamped instead of
		// overflowing into a negative number.
		{"clamped above 92", 200, 7540113804746346429},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Fibonacci(tt.input))
		})
	}
}

// TestFibonacciIsNotExponential guards the fix for the original recursive
// implementation: Fibonacci(50) took minutes, which stalled a worker computing
// the backoff for a heavily retried task.
func TestFibonacciIsNotExponential(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		done <- Fibonacci(90)
	}()

	select {
	case got := <-done:
		assert.Equal(t, 2880067194370816120, got)
	case <-time.After(2 * time.Second):
		t.Fatal("Fibonacci(90) did not complete in time; is it still recursive?")
	}
}

func TestFibonacciNeverNegative(t *testing.T) {
	for n := 0; n < 150; n++ {
		assert.GreaterOrEqual(t, Fibonacci(n), 0, "Fibonacci(%d) overflowed", n)
	}
}

func TestJitterMsecSleep(t *testing.T) {
	tests := []struct {
		name     string
		min, max int
	}{
		{"normal range", 1, 5},
		{"equal bounds", 2, 2},
		{"zero range", 0, 0},
		// The following used to panic inside crypto/rand.Int with
		// "argument to Int is <= 0".
		{"reversed bounds", 5, 1},
		{"negative bounds", -10, -5},
		{"negative min", -5, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				JitterMsecSleep(tt.min, tt.max)
			})
		})
	}
}

func TestJitterMsecSleepStaysWithinBounds(t *testing.T) {
	const min, max = 10, 30

	start := time.Now()
	JitterMsecSleep(min, max)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, time.Duration(min)*time.Millisecond)
	// Generous upper bound: the scheduler, not the jitter, dominates here.
	assert.Less(t, elapsed, 5*time.Second)
}
