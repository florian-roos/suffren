package limiter

import (
	"fmt"
	"time"

	"github.com/florian-roos/suffren/internal/engine"
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
	engine *engine.Engine
	clock  func() time.Time // Injectable clock for deterministic testing
}

func NewLimiter(s *engine.Engine) *Limiter {
	return &Limiter{
		engine: s,
		clock:  time.Now,
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
	consumedLocal := l.getTotalCountInSlidingWindowLocal(identifier, resource, rule)

	if consumedLocal >= rule.Limit {
		return Decision{
			Allowed:   false,
			Current:   rule.Limit,
			Limit:     rule.Limit,
			Remaining: 0,
			ResetAt:   l.clock().Truncate(rule.Window).Add(rule.Window), // The end of the current window
			Error:     nil,
		}
	}

	consumed, ok := l.incrementAndGetTotalCountInSlidingWindow(identifier, resource, value, rule)

	if !ok {
		return Decision{
			Allowed: false,
			Error:   fmt.Errorf("failed to check key: %s, resource: %s", identifier, resource),
		}
	}

	allowed := consumed <= rule.Limit
	var remaining uint64
	if allowed {
		remaining = rule.Limit - consumed
	}

	return Decision{
		Allowed:   allowed,
		Current:   min(consumed, rule.Limit),
		Limit:     rule.Limit,
		Remaining: max(remaining, 0),
		ResetAt:   l.clock().Truncate(rule.Window).Add(rule.Window), // The end of the current window
		Error:     nil,
	}
}

func (l *Limiter) Status(identifier string, resource string, rule Rule) Decision {
	consumed, ok := l.getTotalCountInSlidingWindow(identifier, resource, rule)

	if !ok {
		return Decision{
			Error: fmt.Errorf("failed to get status for key: %s, resource: %s", identifier, resource),
		}
	}

	remaining := rule.Limit - consumed

	return Decision{
		Current:   min(consumed, rule.Limit),
		Limit:     rule.Limit,
		Remaining: max(remaining, 0),
		ResetAt:   l.clock().Truncate(rule.Window).Add(rule.Window), // The end of the current window
		Error:     nil,
	}
}

// increments of the given value and gets the quantity consummed during the sliding window
func (l *Limiter) incrementAndGetTotalCountInSlidingWindow(identifier string, resource string, value uint64, rule Rule) (uint64, bool) {
	now := l.clock()

	currentWindowStart := now.Truncate(rule.Window)
	previousWindowStart := currentWindowStart.Add(-rule.Window)

	currentKey := l.buildKey(identifier, resource, rule, currentWindowStart)
	previousKey := l.buildKey(identifier, resource, rule, previousWindowStart)

	prevCount, ok := l.engine.ValueForKey(previousKey)
	if !ok {
		return 0, false
	}

	currentCount, ok := l.engine.IncrementKey(currentKey, value)
	if !ok {
		return 0, false
	}

	// Calculate the Sliding Window estimate
	// Formula: (Previous Window Count * Overlap Weight) + Current Window Count
	spentTimeInCurrentWindow := now.Sub(currentWindowStart)
	weightOfPreviousWindow := 1.0 - (float64(spentTimeInCurrentWindow) / float64(rule.Window))

	estimatedTotal := float64(prevCount)*weightOfPreviousWindow + float64(currentCount)
	total := uint64(estimatedTotal)

	return total, true
}

// get the value consummed during the sliding window of the given identifier/resource/rule
func (l *Limiter) getTotalCountInSlidingWindow(identifier string, resource string, rule Rule) (uint64, bool) {
	now := l.clock()

	currentWindowStart := now.Truncate(rule.Window)
	previousWindowStart := currentWindowStart.Add(-rule.Window)

	currentKey := l.buildKey(identifier, resource, rule, currentWindowStart)
	previousKey := l.buildKey(identifier, resource, rule, previousWindowStart)

	prevCount, ok := l.engine.ValueForKey(previousKey)
	if !ok {
		return 0, false
	}

	currentCount, ok := l.engine.ValueForKey(currentKey)
	if !ok {
		return 0, false
	}

	// Calculate the Sliding Window estimate
	// Formula: (Previous Window Count * Overlap Weight) + Current Window Count
	spentTimeInCurrentWindow := now.Sub(currentWindowStart)
	weightOfPreviousWindow := 1.0 - (float64(spentTimeInCurrentWindow) / float64(rule.Window))

	estimatedTotal := float64(prevCount)*weightOfPreviousWindow + float64(currentCount)
	total := uint64(estimatedTotal)

	return total, true
}

// get the consumed value the node sees locally during the sliding window of the given identifier/resource/rule
func (l *Limiter) getTotalCountInSlidingWindowLocal(identifier string, resource string, rule Rule) uint64 {
	now := l.clock()

	currentWindowStart := now.Truncate(rule.Window)
	previousWindowStart := currentWindowStart.Add(-rule.Window)

	currentKey := l.buildKey(identifier, resource, rule, currentWindowStart)
	previousKey := l.buildKey(identifier, resource, rule, previousWindowStart)

	prevCount := l.engine.ValueForKeyLocal(previousKey)

	currentCount := l.engine.ValueForKeyLocal(currentKey)

	// Calculate the Sliding Window estimate
	// Formula: (Previous Window Count * Overlap Weight) + Current Window Count
	spentTimeInCurrentWindow := now.Sub(currentWindowStart)
	weightOfPreviousWindow := 1.0 - (float64(spentTimeInCurrentWindow) / float64(rule.Window))

	estimatedTotal := float64(prevCount)*weightOfPreviousWindow + float64(currentCount)
	total := uint64(estimatedTotal)

	return total
}
