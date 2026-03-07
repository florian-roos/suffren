package suffren

import (
	"suffren/internal/crdt"
	"suffren/pkg/config"
	"sync"
	"testing"
	"time"
)

// Helpers

func configForTest() *config.Config {
	cfg := config.DefaultConfig()
	return cfg
}

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

// startCluster creates and starts all 3 nodes. Returns them and a cleanup func.
func startCluster(t *testing.T, peers map[crdt.NodeId]string) (s1, s2, s3 *Suffren) {
	t.Helper()
	cfg := configForTest()

	var ports []string
	var ids []crdt.NodeId
	for id, addr := range peers {
		ids = append(ids, id)
		// extract port from "localhost:PORT"
		ports = append(ports, addr[len("localhost:"):])
	}

	s1 = NewSuffren(ids[0], ports[0], peers, cfg)
	s2 = NewSuffren(ids[1], ports[1], peers, cfg)
	s3 = NewSuffren(ids[2], ports[2], peers, cfg)

	for _, s := range []*Suffren{s1, s2, s3} {
		if err := s.Start(); err != nil {
			t.Fatalf("failed to start node: %v", err)
		}
	}

	t.Cleanup(func() {
		s1.Stop()
		s2.Stop()
		s3.Stop()
	})

	return s1, s2, s3
}

// waitForConvergence polls until all nodes report the expected value or times out.
func waitForConvergence(t *testing.T, expected uint64, nodes ...*Suffren) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allConverged := true
		for _, n := range nodes {
			val, ok := n.Value()
			if !ok || val != expected {
				allConverged = false
				break
			}
		}
		if allConverged {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for i, n := range nodes {
		val, ok := n.Value()
		t.Errorf("node %d: expected %d, got %d (ok=%v)", i, expected, val, ok)
	}
	t.Fatalf("nodes did not converge to %d within 5s", expected)
}

// Tests

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
