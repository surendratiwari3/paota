package utils

import (
	"crypto/rand"
	"math/big"
	"time"
)

// Fibonacci calculates the nth Fibonacci number.
func Fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return Fibonacci(n-1) + Fibonacci(n-2)
}

func JitterMsecSleep(min, max int) {
	// random number in [0, max-min]
	nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	n := int(nBig.Int64()) + min
	time.Sleep(time.Duration(n) * time.Millisecond)
}
