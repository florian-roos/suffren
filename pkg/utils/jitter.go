package utils

import (
	"math/rand"
	"time"
)

// Jitter returns a random duration in [-d/5, +d/5] to spread retries.
func Jitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(d/5)*2) - int64(d/5))
}
