package retry

import (
	"log/slog"
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
		slog.Warn("Retry attempt failed", "attempt", attempt, "maxAttempts", config.MaxAttempts, "delay", delay, "error", lastErr)

		if attempt < config.MaxAttempts {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * config.Backoff)
		}
	}
	return lastErr
}
