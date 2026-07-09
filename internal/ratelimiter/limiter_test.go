package ratelimiter

import (
	"sync"
	"testing"
	"time"

	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/testutils"
	"github.com/florian-roos/suffren/pkg/config"
	"github.com/florian-roos/suffren/pkg/suffren"
)

func configForTest() *config.Config {
	return config.DefaultConfig()
}

func peers3() map[crdt.NodeId]string {
	return testutils.GeneratePeers3()
}

func startCluster(tb testing.TB, peers map[crdt.NodeId]string) (s1, s2, s3 *suffren.Suffren) {
	tb.Helper()
	cfg := configForTest()

	var ports []string
	var ids []crdt.NodeId
	for id, addr := range peers {
		ids = append(ids, id)
		ports = append(ports, addr[len("localhost:"):])
	}

	s1 = suffren.NewSuffren(ids[0], peers, cfg)
	s2 = suffren.NewSuffren(ids[1], peers, cfg)
	s3 = suffren.NewSuffren(ids[2], peers, cfg)

	var wg sync.WaitGroup
	for _, s := range []*suffren.Suffren{s1, s2, s3} {
		wg.Add(1)
		go func(node *suffren.Suffren) {
			defer wg.Done()
			if err := node.Start(); err != nil {
				tb.Errorf("failed to start node: %v", err)
			}
		}(s)
	}
	wg.Wait()

	tb.Cleanup(func() {
		s1.Stop()
		s2.Stop()
		s3.Stop()
	})

	return s1, s2, s3
}

func TestLimiter_allows_under_limit(t *testing.T) {
	// GIVEN: a 3-node cluster and a rule allowing 5 requests per minute
	// WHEN: we make 5 requests under the limit
	// THEN: all requests are allowed, and Current/Remaining are updated correctly
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	rule := Rule{Limit: 5, Window: 1 * time.Minute}

	for i := 1; i <= 5; i++ {
		decision := limiter.Check("userA", "api", 1, rule)
		if !decision.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
		if decision.Current != uint64(i) {
			t.Errorf("expected current to be %d, got %d", i, decision.Current)
		}
		if decision.Remaining != uint64(5-i) {
			t.Errorf("expected remaining to be %d, got %d", 5-i, decision.Remaining)
		}
	}
}

func TestLimiter_denies_over_limit(t *testing.T) {
	// GIVEN: a 3-node cluster and a rule allowing 3 requests per minute
	// WHEN: we make 4 requests
	// THEN: the first 3 are allowed, and the 4th is denied with Remaining = 0
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	rule := Rule{Limit: 3, Window: 1 * time.Minute}

	// 1, 2, 3 should pass
	for i := 1; i <= 3; i++ {
		decision := limiter.Check("userB", "api", 1, rule)
		if !decision.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}

	// 4th should be denied
	decision := limiter.Check("userB", "api", 1, rule)
	if decision.Allowed {
		t.Fatal("expected 4th request to be denied")
	}
	if decision.Remaining != 0 {
		t.Errorf("expected remaining to be 0 when denied, got %d", decision.Remaining)
	}
}

func TestLimiter_sliding_window_weighs_previous_window(t *testing.T) {
	// GIVEN: a 3-node cluster and a sliding window rule of 100 req/min
	// WHEN: we consume 60 requests in the previous window, and 10 requests at 15s into the current window
	// THEN: the estimated total correctly weights the previous window (60 * 75% = 45) + 10 = 55
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	baseTime := time.Date(2026, 7, 5, 14, 0, 0, 0, time.UTC)
	rule := Rule{Limit: 100, Window: 1 * time.Minute}

	// Simulate the previous minute (14:00:xx). The user consumed 60 requests.
	limiter.clock = func() time.Time {
		return baseTime.Add(30 * time.Second) // 14:00:30
	}
	limiter.Check("userC", "api", 60, rule)

	// Step 2: Now we are in the CURRENT minute (14:01:xx).
	// We are exactly 15 seconds into the new minute.
	// The overlap weight of the previous minute should be: 1 - (15/60) = 0.75
	// So the estimated carry-over from the previous window is: 60 * 0.75 = 45.
	limiter.clock = func() time.Time {
		return baseTime.Add(1 * time.Minute).Add(15 * time.Second) // 14:01:15
	}

	// We consume 10 requests now.
	// Total estimate should be: 45 (from prev) + 10 (current) = 55.
	decision := limiter.Check("userC", "api", 10, rule)

	if !decision.Allowed {
		t.Fatal("expected request to be allowed")
	}
	if decision.Current != 55 {
		t.Errorf("expected sliding window calculation to be 55, got %d", decision.Current)
	}
	if decision.Remaining != 45 {
		t.Errorf("expected remaining to be 45, got %d", decision.Remaining)
	}
}

func TestLimiter_different_identifiers_and_resources_are_independent(t *testing.T) {
	// GIVEN: a 3-node cluster and a rule
	// WHEN: different users consume different resources
	// THEN: the counters remain strictly independent
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	rule := Rule{Limit: 10, Window: 1 * time.Minute}

	// user1 consumes 5 "api"
	dec1 := limiter.Check("user1", "api", 5, rule)
	if dec1.Current != 5 {
		t.Errorf("expected user1 api to be 5, got %d", dec1.Current)
	}

	// user1 consumes 2 "uploads" (different resource, should be independent)
	dec2 := limiter.Check("user1", "uploads", 2, rule)
	if dec2.Current != 2 {
		t.Errorf("expected user1 uploads to be 2, got %d", dec2.Current)
	}

	// user2 consumes 8 "api" (different user, should be independent)
	dec3 := limiter.Check("user2", "api", 8, rule)
	if dec3.Current != 8 {
		t.Errorf("expected user2 api to be 8, got %d", dec3.Current)
	}
}

func TestLimiter_Status_returns_current_count_without_incrementing(t *testing.T) {
	// GIVEN: a 3-node cluster and a rule allowing 10 requests per minute
	// WHEN: we make 3 requests, then call Status
	// THEN: Status returns 3 without incrementing the counter
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	rule := Rule{Limit: 10, Window: 1 * time.Minute}

	// Make 3 requests
	limiter.Check("user_status", "api", 3, rule)

	// Call Status
	status := limiter.Status("user_status", "api", rule)

	if status.Error != nil {
		t.Fatalf("expected no error, got %v", status.Error)
	}
	if status.Current != 3 {
		t.Errorf("expected status to return 3, got %d", status.Current)
	}
	if status.Remaining != 7 {
		t.Errorf("expected remaining to be 7, got %d", status.Remaining)
	}

	// Verify it didn't increment
	status2 := limiter.Status("user_status", "api", rule)
	if status2.Current != 3 {
		t.Errorf("expected status to remain 3, got %d", status2.Current)
	}
}

func TestLimiter_Status_applies_sliding_window_weight(t *testing.T) {
	// GIVEN: a 3-node cluster and a sliding window rule
	// WHEN: we consume requests in the previous window and check Status in the current window
	// THEN: Status applies the overlap weight formula correctly without incrementing
	s1, _, _ := startCluster(t, peers3())
	limiter := NewLimiter(s1)

	baseTime := time.Date(2026, 7, 5, 14, 0, 0, 0, time.UTC)
	rule := Rule{Limit: 100, Window: 1 * time.Minute}

	// Simulate the previous minute (14:00:xx). Consume 60 requests.
	limiter.clock = func() time.Time {
		return baseTime.Add(30 * time.Second) // 14:00:30
	}
	limiter.Check("user_sliding_status", "api", 60, rule)

	// Now we are in the CURRENT minute (14:01:xx). 15 seconds into the new minute.
	// Overlap weight: 1 - (15/60) = 0.75
	// Estimated carry-over: 60 * 0.75 = 45.
	limiter.clock = func() time.Time {
		return baseTime.Add(1 * time.Minute).Add(15 * time.Second) // 14:01:15
	}

	// Call Status
	status := limiter.Status("user_sliding_status", "api", rule)

	if status.Error != nil {
		t.Fatalf("expected no error, got %v", status.Error)
	}
	if status.Current != 45 {
		t.Errorf("expected sliding status to be 45, got %d", status.Current)
	}
}
