package utils

import (
	"crypto/rand"
	"math/big"
	mathrand "math/rand"
	"time"
)

// maxFibonacciN is the largest input for which Fibonacci is computed exactly.
// Fibonacci(92) is the biggest value that still fits in a signed 64-bit int, so
// anything beyond that is clamped to avoid overflowing into negative numbers.
const maxFibonacciN = 92

// Fibonacci calculates the nth Fibonacci number.
//
// The computation is iterative: the previous recursive implementation was
// O(2^n), so a caller passing a moderately large retry count (for example a
// task that has been retried 45 times) would peg a CPU for minutes before
// returning. Negative inputs return 0 and inputs above maxFibonacciN are
// clamped so the result never overflows.
func Fibonacci(n int) int {
	if n <= 0 {
		return 0
	}
	if n == 1 {
		return 1
	}
	if n > maxFibonacciN {
		n = maxFibonacciN
	}

	prev, curr := 0, 1
	for i := 2; i <= n; i++ {
		prev, curr = curr, prev+curr
	}
	return curr
}

// JitterMsecSleep sleeps for a random duration in [min, max] milliseconds.
//
// Inputs are normalised rather than trusted: reversed bounds are swapped and
// negative bounds are floored at zero. Previously a reversed or equal-and-
// negative range made crypto/rand.Int panic with "argument to Int is <= 0",
// and a rand.Reader failure dereferenced a nil *big.Int.
func JitterMsecSleep(min, max int) {
	if min > max {
		min, max = max, min
	}
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if max == min {
		if max > 0 {
			time.Sleep(time.Duration(max) * time.Millisecond)
		}
		return
	}

	// random number in [0, max-min]
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil || nBig == nil {
		// crypto/rand is unavailable; jitter is a best-effort optimisation so
		// fall back to the math/rand source instead of panicking.
		time.Sleep(time.Duration(mathrand.Intn(max-min+1)+min) * time.Millisecond)
		return
	}
	n := int(nBig.Int64()) + min
	time.Sleep(time.Duration(n) * time.Millisecond)
}
