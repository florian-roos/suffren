package p2p

import (
	"log"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
	Backoff     float64
}

func Retry(config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.Delay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("[RETRY] Attempt %d/%d failed. Retry in %v: %v", attempt, config.MaxAttempts, delay, lastErr)

		if attempt < config.MaxAttempts {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * config.Backoff)
		}
	}
	return lastErr
}
