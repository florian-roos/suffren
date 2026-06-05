package suffren

import (
	"sync"
	"testing"

	"github.com/florian-roos/suffren/internal/crdt"
)

func TestSuffren_node_initial_value_is_zero(t *testing.T) {
	// GIVEN: a 3-node cluster (quorum = 2)
	// THEN:  its initial value is zero (the empty GCounter)
	s1, _, _ := startCluster(t, peers3())

	val, ok := s1.Value()
	if !ok {
		t.Fatal("Value() timed out on a fresh cluster")
	}
	if val != 0 {
		t.Errorf("expected initial value to be 0, got %d", val)
	}
}

func TestSuffren_increment_increses_value(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN:  we call Increment() n times on one node
	// THEN:  Value() returns n
	s1, _, _ := startCluster(t, peers3bis())

	increments := 5
	for i := 0; i < increments; i++ {
		_, ok := s1.Increment()
		if !ok {
			t.Fatalf("Increment() #%d timed out", i+1)
		}
	}

	val, ok := s1.Value()
	if !ok {
		t.Fatal("Value() timed out")
	}
	if val != uint64(increments) {
		t.Errorf("expected value to be %d after %d increments, got %d", increments, increments, val)
	}
}

func TestSuffren_concurrent_increments_converge_to_the_same_value(t *testing.T) {
	// GIVEN: 3 nodes that run in parallel
	// WHEN: The nodes Increment() concurrently with goroutines
	// THEN: The nodes converge toward the same value
	// This ensures that no value is lost even with contention

	peers := peers3bis()
	s1, s2, s3 := startCluster(t, peers)

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

	peers := peers3()

	cfg := configForTest()

	// Start only N1 and N2 initially — extract their ports from the peers map.
	s1 := NewSuffren(crdt.NodeId("N1"), peers["N1"][len("localhost:"):], peers, cfg)
	if err := s1.Start(); err != nil {
		t.Fatalf("failed to start node 1: %v", err)
	}
	defer s1.Stop()

	s2 := NewSuffren(crdt.NodeId("N2"), peers["N2"][len("localhost:"):], peers, cfg)
	if err := s2.Start(); err != nil {
		t.Fatalf("failed to start node 2: %v", err)
	}
	defer s2.Stop()

	// Increment a few times before the 3rd node starts
	for i := 0; i < 10; i++ {
		s1.Increment()
		s2.Increment()
	}

	s3 := NewSuffren(crdt.NodeId("N3"), peers["N3"][len("localhost:"):], peers, cfg)
	if err := s3.Start(); err != nil {
		t.Fatalf("failed to start node 3: %v", err)
	}
	defer s3.Stop()

	waitForConvergence(t, 20, s1, s2, s3)
}




