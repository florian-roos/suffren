package suffren

import (
	"suffren/internal/crdt"
	"sync"
	"testing"
	"time"
)

// Helpers

func peers3() map[crdt.NodeId]string {
	return map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
		"N3": "localhost:8003",
	}
}

func peers3bis() map[crdt.NodeId]string {
	return map[crdt.NodeId]string{
		"N1": "localhost:8011",
		"N2": "localhost:8012",
		"N3": "localhost:8013",
	}
}

// waitForConvergence polls until all nodes report the expected value or times out.
// Needed because convergence happens asynchronously over TCP.
func waitForConvergence(t *testing.T, expected uint64, nodes ...*Suffren) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allConverged := true
		for _, n := range nodes {
			if n.Value() != expected {
				allConverged = false
				break
			}
		}
		if allConverged {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Timeout — report what each node actually has
	for i, n := range nodes {
		t.Errorf("node %d: expected %d, got %d", i, expected, n.Value())
	}
	t.Fatalf("nodes did not converge to %d within 5s", expected)
}

// Tests

func TestSuffren_node_initial_value_is_zero(t *testing.T) {
	// GIVEN: a newly created Suffren node
	// THEN:  its initial value is zero (the empty GCounter)
	suffren, err := NewSuffren("N1", "8001", peers3())
	if err != nil {
		t.Fatalf("failed to create Suffren node: %v", err)
	}
	defer suffren.Stop()

	if val := suffren.Value(); val != 0 {
		t.Errorf("expected initial value to be 0, got %d", val)
	}
}

func TestSuffren_increment_increses_value(t *testing.T) {
	// GIVEN: a Suffren node
	// WHEN:  we call Increment() n times
	// THEN:  Value() returns n
	suffren, err := NewSuffren(crdt.NodeId("N1"), "8001", peers3())
	if err != nil {
		t.Fatalf("failed to create Suffren node: %v", err)
	}
	defer suffren.Stop()

	increments := 5
	for i := 0; i < increments; i++ {
		suffren.Increment()
	}

	if val := suffren.Value(); val != uint64(increments) {
		t.Errorf("expected value to be %d after %d increments, got %d", increments, increments, val)
	}
}

func TestSuffren_concurrent_increments_converge_to_the_same_value(t *testing.T) {
	// GIVEN: 3 nodes that run in parrallel
	// WHEN: The nodes Increment() concurrently with goroutines
	// THEN: The nodes converge toward the same value
	// This ensures that no value is lost even with contention

	peers := peers3bis()

	s1, err := NewSuffren(crdt.NodeId("N1"), "8011", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 1: %v", err)
	}
	defer s1.Stop()
	s2, err := NewSuffren(crdt.NodeId("N2"), "8012", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 2: %v", err)
	}
	defer s2.Stop()
	s3, err := NewSuffren(crdt.NodeId("N3"), "8013", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 3: %v", err)
	}
	defer s3.Stop()

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); s1.Increment() }()
		go func() { defer wg.Done(); s2.Increment() }()
		go func() { defer wg.Done(); s3.Increment() }()
	}

	wg.Wait()

	waitForConvergence(t, 300, s1, s2, s3)
}

func TestSuffren_late_joining_node_catches_up(t *testing.T) {
	// GIVEN: 2 nodes that run in parallel, and a 3rd node that starts later
	// WHEN:  the first 2 nodes Increment() a few times before the 3rd node starts
	// THEN:  all nodes eventually converge to the same value, including the late-joining node
	// This validates that late-joining nodes can catch up through periodic proposals.

	peers := peers3()

	s1, err := NewSuffren(crdt.NodeId("N1"), "8001", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 1: %v", err)
	}
	defer s1.Stop()
	s2, err := NewSuffren(crdt.NodeId("N2"), "8002", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 2: %v", err)
	}
	defer s2.Stop()

	// Increment a few times before the 3rd node starts
	for i := 0; i < 10; i++ {
		s1.Increment()
		s2.Increment()
	}

	s3, err := NewSuffren(crdt.NodeId("N3"), "8003", peers)
	if err != nil {
		t.Fatalf("failed to create Suffren node 3: %v", err)
	}
	defer s3.Stop()

	waitForConvergence(t, 20, s1, s2, s3)
}
