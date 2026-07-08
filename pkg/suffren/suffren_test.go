package suffren

import (
	"sync"
	"testing"

	"github.com/florian-roos/suffren/internal/crdt"
)

func TestSuffren_ValueForKey_on_an_unknown_key(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN: we query an unknown key
	// THEN: it returns 0
	s1, _, _ := startCluster(t, peers3())

	val, ok := s1.ValueForKey("unknown_key")
	if !ok {
		t.Fatal("ValueForKey() timed out on a fresh cluster")
	}
	if val != 0 {
		t.Errorf("expected initial value to be 0, got %d", val)
	}
}

func TestSuffren_increment_one_key_does_not_affect_others(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN: we increment "test2_key_A" multiple times
	// THEN: "test2_key_A" has the correct value, and "test2_key_B" remains 0
	s1, _, _ := startCluster(t, peers3bis())

	increments := 5
	for i := 0; i < increments; i++ {
		_, ok := s1.IncrementKey("test2_key_A", 1)
		if !ok {
			t.Fatalf("IncrementKey() #%d timed out", i+1)
		}
	}

	valA, okA := s1.ValueForKey("test2_key_A")
	if !okA {
		t.Fatal("ValueForKey(test2_key_A) timed out")
	}
	if valA != uint64(increments) {
		t.Errorf("expected test2_key_A to be %d, got %d", increments, valA)
	}

	valB, okB := s1.ValueForKey("test2_key_B")
	if !okB {
		t.Fatal("ValueForKey(test2_key_B) timed out")
	}
	if valB != 0 {
		t.Errorf("expected test2_key_B to be 0, got %d", valB)
	}
}

func TestSuffren_concurrent_increments_on_multiple_keys(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN: the nodes increment multiple different keys concurrently
	// THEN: all nodes converge to the correct values for all keys
	peers := peers3bis()
	s1, s2, s3 := startCluster(t, peers)

	wg := sync.WaitGroup{}

	// We'll increment test3_key_A 100 times, test3_key_B 100 times, test3_key_C 100 times
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); s1.IncrementKey("test3_key_A", 1) }()
		go func() { defer wg.Done(); s2.IncrementKey("test3_key_B", 1) }()
		go func() { defer wg.Done(); s3.IncrementKey("test3_key_C", 1) }()
	}

	wg.Wait()

	waitForConvergence(t, "test3_key_A", 100, s1, s2, s3)
	waitForConvergence(t, "test3_key_B", 100, s1, s2, s3)
	waitForConvergence(t, "test3_key_C", 100, s1, s2, s3)
}

func TestSuffren_concurrent_increments_on_same_key(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN: the nodes increment the SAME key concurrently
	// THEN: the cluster converges without dropping any increments
	peers := peers3()
	s1, s2, s3 := startCluster(t, peers)

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); s1.IncrementKey("shared_key", 1) }()
		go func() { defer wg.Done(); s2.IncrementKey("shared_key", 1) }()
		go func() { defer wg.Done(); s3.IncrementKey("shared_key", 1) }()
	}

	wg.Wait()

	waitForConvergence(t, "shared_key", 300, s1, s2, s3)
}

func TestSuffren_late_joining_node_catches_up_with_all_keys(t *testing.T) {
	// GIVEN: 2 nodes that run in parallel, and a 3rd node that starts later
	// WHEN: the first 2 nodes increment various keys
	// THEN: the 3rd node fetches all keys and converges perfectly
	peers := peers3()
	cfg := configForTest()

	// Start N1 and N2 concurrently so they can form a quorum (2 out of 3)
	s1 := NewSuffren(crdt.NodeId("N1"), peers, cfg)
	s2 := NewSuffren(crdt.NodeId("N2"), peers, cfg)

	var wgStart sync.WaitGroup
	wgStart.Add(2)
	go func() {
		defer wgStart.Done()
		if err := s1.Start(); err != nil {
			t.Errorf("node 1 failed: %v", err)
		}
	}()
	go func() {
		defer wgStart.Done()
		if err := s2.Start(); err != nil {
			t.Errorf("node 2 failed: %v", err)
		}
	}()
	wgStart.Wait()

	defer s1.Stop()
	defer s2.Stop()

	// N1 and N2 increment different keys
	for i := 0; i < 10; i++ {
		s1.IncrementKey("key_1", 1)
		s2.IncrementKey("key_2", 1)
	}

	// Now start N3
	s3 := NewSuffren(crdt.NodeId("N3"), peers, cfg)
	if err := s3.Start(); err != nil {
		t.Fatalf("failed to start node 3: %v", err)
	}
	defer s3.Stop()

	waitForConvergence(t, "key_1", 10, s1, s2, s3)
	waitForConvergence(t, "key_2", 10, s1, s2, s3)
}
