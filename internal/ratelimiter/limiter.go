package ratelimiter

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/florian-roos/suffren/pkg/suffren"
)

// Decision represents the outcome of a rate limit check.
type Decision struct {
	Allowed   bool
	Current   uint64
	Limit     uint64
	Remaining uint64
	ResetAt   time.Time
	Error     error // If the cluster timed out, this will be populated.
}

// Rule defines the rate limiting policy.
type Rule struct {
	Limit  uint64
	Window time.Duration
}

type Limiter struct {
	suffren *suffren.Suffren
	clock   func() time.Time // Injectable clock for deterministic testing
}

func NewLimiter(s *suffren.Suffren) *Limiter {
	return &Limiter{
		suffren: s,
		clock:   time.Now,
	}
}

// Constructs a deterministic key based on the window's start time.
// Example: "user123:api_requests:60s:2026-07-05T16:00:00Z"
func (l *Limiter) buildKey(identifier string, resource string, rule Rule, windowStart time.Time) string {
	timeStr := windowStart.UTC().Format(time.RFC3339)
	return fmt.Sprintf("%s:%s:%ds:%s", identifier, resource, int(rule.Window.Seconds()), timeStr)
}

// Evaluates whether the request is allowed (sliding window counter).
// It executes in a single atomic-like operation.
func (l *Limiter) Check(identifier string, resource string, value uint64, rule Rule) Decision {
	consumed, err := l.incrementAndGetTotalCountInSlidingWindow(identifier, resource, value, rule)

	if err != nil {
		return Decision{
			Allowed: false,
			Error:   err,
		}
	}

	allowed := consumed <= rule.Limit
	var remaining uint64
	if allowed {
		remaining = rule.Limit - consumed
	}

	return Decision{
		Allowed:   allowed,
		Current:   consumed,
		Limit:     rule.Limit,
		Remaining: remaining,
		ResetAt:   l.clock().Truncate(rule.Window).Add(rule.Window), // The end of the current window
		Error:     nil,
	}
}

func (l *Limiter) Status(identifier string, resource string, rule Rule) Decision {
	consumed, err := l.getTotalCountInSlidingWindow(identifier, resource, rule)

	if err != nil {
		return Decision{
			Error: err,
		}
	}

	remaining := rule.Limit - consumed

	return Decision{
		Current:   consumed,
		Limit:     rule.Limit,
		Remaining: remaining,
		ResetAt:   l.clock().Truncate(rule.Window).Add(rule.Window), // The end of the current window
		Error:     nil,
	}
}

// increments of the given value and gets the quantity consummed during the sliding window
func (l *Limiter) incrementAndGetTotalCountInSlidingWindow(identifier string, resource string, value uint64, rule Rule) (uint64, error) {
	now := l.clock()

	currentWindowStart := now.Truncate(rule.Window)
	previousWindowStart := currentWindowStart.Add(-rule.Window)

	currentKey := l.buildKey(identifier, resource, rule, currentWindowStart)
	previousKey := l.buildKey(identifier, resource, rule, previousWindowStart)

	prevCount, ok := l.suffren.ValueForKey(previousKey)
	if !ok {
		err := fmt.Errorf("timeout reading previous window from cluster")
		slog.Error("Rate limiter cluster timeout", "key", previousKey, "error", err)
		return 0, err
	}

	currentCount, ok := l.suffren.IncrementKey(currentKey, value)
	if !ok {
		err := fmt.Errorf("timeout incrementing current window in cluster")
		slog.Error("Rate limiter cluster timeout", "key", currentKey, "error", err)
		return 0, err
	}

	// Calculate the Sliding Window estimate
	// Formula: (Previous Window Count * Overlap Weight) + Current Window Count
	spentTimeInCurrentWindow := now.Sub(currentWindowStart)
	weightOfPreviousWindow := 1.0 - (float64(spentTimeInCurrentWindow) / float64(rule.Window))

	estimatedTotal := float64(prevCount)*weightOfPreviousWindow + float64(currentCount)
	total := uint64(estimatedTotal)

	return total, nil
}

func (l *Limiter) getTotalCountInSlidingWindow(identifier string, resource string, rule Rule) (uint64, error) {
	now := l.clock()

	currentWindowStart := now.Truncate(rule.Window)
	previousWindowStart := currentWindowStart.Add(-rule.Window)

	currentKey := l.buildKey(identifier, resource, rule, currentWindowStart)
	previousKey := l.buildKey(identifier, resource, rule, previousWindowStart)

	prevCount, ok := l.suffren.ValueForKey(previousKey)
	if !ok {
		err := fmt.Errorf("timeout reading previous window from cluster")
		slog.Error("Rate limiter cluster timeout", "key", previousKey, "error", err)
		return 0, err
	}

	currentCount, ok := l.suffren.ValueForKey(currentKey)
	if !ok {
		err := fmt.Errorf("timeout reading current window in cluster")
		slog.Error("Rate limiter cluster timeout", "key", currentKey, "error", err)
		return 0, err
	}

	// Calculate the Sliding Window estimate
	// Formula: (Previous Window Count * Overlap Weight) + Current Window Count
	spentTimeInCurrentWindow := now.Sub(currentWindowStart)
	weightOfPreviousWindow := 1.0 - (float64(spentTimeInCurrentWindow) / float64(rule.Window))

	estimatedTotal := float64(prevCount)*weightOfPreviousWindow + float64(currentCount)
	total := uint64(estimatedTotal)

	return total, nil
}
