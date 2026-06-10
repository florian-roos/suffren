package crdt

import (
	"reflect"
	"testing"
)

func TestGCounter_NewGCounter_initializes_all_to_zero(t *testing.T) {
	// GIVEN: a 2-node cluster
	// WHEN: the GCounter is created
	// THEN: the count of the GCounter is 0
	nodeIds := []NodeId{"node1", "node2"}
	g := NewGCounter(nodeIds)

	if len(g.Counts) != 2 {
		t.Errorf("Expected length 2, got %d", len(g.Counts))
	}
	for _, id := range nodeIds {
		if g.Counts[id] != 0 {
			t.Errorf("Expected count 0 for node %s, got %d", id, g.Counts[id])
		}
	}
}

func TestGCounter_Increment_increases_correct_node(t *testing.T) {
	// GIVEN: a 2-node GCounter
	// WHEN: we call Increment() on a node
	// THEN: the count of this node is 1 and the count of the other one is 0
	g := NewGCounter([]NodeId{"node1", "node2"})
	g.Increment("node1")

	if g.Counts["node1"] != 1 {
		t.Errorf("Expected count 1 for node1, got %d", g.Counts["node1"])
	}
	if g.Counts["node2"] != 0 {
		t.Errorf("Expected count 0 for node2, got %d", g.Counts["node2"])
	}
}

func TestGCounter_Value_returns_sum_of_all_counts(t *testing.T) {
	// GIVEN: a GCounter with multiple nodes having some counts
	// WHEN: we call Value()
	// THEN: it returns the total sum of all the nodes counts
	g := NewGCounter([]NodeId{"node1", "node2", "node3"})
	g.Counts["node1"] = 5
	g.Counts["node2"] = 3
	g.Counts["node3"] = 2

	if v := g.Value(); v != 10 {
		t.Errorf("Expected value 10, got %d", v)
	}
}

func TestGCounter_Join_returns_component_wise_max(t *testing.T) {
	// GIVEN: two GCounters with some overlapping and disjoint state
	// WHEN: we Join() them
	// THEN: it returns a new GCounter with the component-wise max, without mutating the original
	g1 := NewGCounter([]NodeId{"node1", "node2"})
	g1.Counts["node1"] = 5
	g1.Counts["node2"] = 2

	g2 := NewGCounter([]NodeId{"node2", "node3"})
	g2.Counts["node2"] = 7
	g2.Counts["node3"] = 1

	result := g1.Join(g2).(*GCounter)

	expected := map[NodeId]uint64{"node1": 5, "node2": 7, "node3": 1}
	if !reflect.DeepEqual(result.Counts, expected) {
		t.Errorf("Expected counts %v, got %v", expected, result.Counts)
	}
	
	if g1.Counts["node2"] != 2 {
		t.Errorf("Join should not mutate the receiver")
	}
}

func TestGCounter_MergeInPlace_mutates_receiver(t *testing.T) {
	// GIVEN: two GCounters with some overlapping and disjoint state
	// WHEN: we MergeInPlace() the second into the first
	// THEN: the first GCounter is mutated to hold the component-wise max
	g1 := NewGCounter([]NodeId{"node1", "node2"})
	g1.Counts["node1"] = 5
	g1.Counts["node2"] = 2

	g2 := NewGCounter([]NodeId{"node2", "node3"})
	g2.Counts["node2"] = 7
	g2.Counts["node3"] = 1

	g1.MergeInPlace(g2)

	expected := map[NodeId]uint64{"node1": 5, "node2": 7, "node3": 1}
	if !reflect.DeepEqual(g1.Counts, expected) {
		t.Errorf("Expected counts %v, got %v", expected, g1.Counts)
	}
}

func TestGCounter_IsIn_partial_order(t *testing.T) {
	// GIVEN: a set of GCounter pairs (g1, g2)
	// WHEN: we evaluate g1.IsIn(g2)
	// THEN: it returns true if g1 is less than or equal to g2 for all components, false otherwise
	tests := []struct {
		name     string
		g1Counts map[NodeId]uint64
		g2Counts map[NodeId]uint64
		expected bool
	}{
		{
			name:     "subset of keys and smaller values",
			g1Counts: map[NodeId]uint64{"node1": 2},
			g2Counts: map[NodeId]uint64{"node1": 5, "node2": 1},
			expected: true,
		},
		{
			name:     "same keys and same values",
			g1Counts: map[NodeId]uint64{"node1": 5},
			g2Counts: map[NodeId]uint64{"node1": 5},
			expected: true,
		},
		{
			name:     "not in, greater value",
			g1Counts: map[NodeId]uint64{"node1": 6},
			g2Counts: map[NodeId]uint64{"node1": 5},
			expected: false,
		},
		{
			name:     "not in, missing key in other",
			g1Counts: map[NodeId]uint64{"node1": 1, "node3": 1},
			g2Counts: map[NodeId]uint64{"node1": 5},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g1 := &GCounter{Counts: tc.g1Counts}
			g2 := &GCounter{Counts: tc.g2Counts}
			if result := g1.IsIn(g2); result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGCounter_StrictlyIsIn(t *testing.T) {
	// GIVEN: a set of GCounter pairs (g1, g2)
	// WHEN: we evaluate g1.StrictlyIsIn(g2)
	// THEN: it returns true if g1 is less than or equal to g2 for all components AND strictly less in at least one
	tests := []struct {
		name     string
		g1Counts map[NodeId]uint64
		g2Counts map[NodeId]uint64
		expected bool
	}{
		{
			name:     "strictly less in one component",
			g1Counts: map[NodeId]uint64{"node1": 2},
			g2Counts: map[NodeId]uint64{"node1": 5, "node2": 1},
			expected: true,
		},
		{
			name:     "equal is not strictly less",
			g1Counts: map[NodeId]uint64{"node1": 5},
			g2Counts: map[NodeId]uint64{"node1": 5},
			expected: false,
		},
		{
			name:     "greater is not strictly less",
			g1Counts: map[NodeId]uint64{"node1": 6},
			g2Counts: map[NodeId]uint64{"node1": 5},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g1 := &GCounter{Counts: tc.g1Counts}
			g2 := &GCounter{Counts: tc.g2Counts}
			if result := g1.StrictlyIsIn(g2); result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGCounter_Equals(t *testing.T) {
	// GIVEN: several GCounters
	// WHEN: we compare them using Equals()
	// THEN: it returns true only if they have the exact same keys and counts
	g1 := &GCounter{Counts: map[NodeId]uint64{"node1": 5, "node2": 3}}
	g2 := &GCounter{Counts: map[NodeId]uint64{"node1": 5, "node2": 3}}
	g3 := &GCounter{Counts: map[NodeId]uint64{"node1": 5}}

	if !g1.Equals(g2) {
		t.Errorf("Expected g1 to equal g2")
	}
	if g1.Equals(g3) {
		t.Errorf("Expected g1 to not equal g3")
	}
}

func TestGCounter_Copy_is_deep(t *testing.T) {
	// GIVEN: a GCounter
	// WHEN: we create a Copy() of it
	// THEN: the copy is equal to the original but modifying it does not affect the original
	g1 := &GCounter{Counts: map[NodeId]uint64{"node1": 5}}
	g2 := g1.Copy()

	if !g1.Equals(g2) {
		t.Errorf("Copy should be equal to original")
	}

	g2.Counts["node1"] = 10
	if g1.Counts["node1"] == 10 {
		t.Errorf("Copy should be deep, modifying copy mutated original")
	}
}

func TestGCounter_Bottom_returns_all_zeros(t *testing.T) {
	// GIVEN: a GCounter with some counts
	// WHEN: we request its Bottom()
	// THEN: it returns a new GCounter with the same nodes but all counts set to 0
	g1 := &GCounter{Counts: map[NodeId]uint64{"node1": 5, "node2": 3}}
	bottom := g1.Bottom().(*GCounter)

	expected := map[NodeId]uint64{"node1": 0, "node2": 0}
	if !reflect.DeepEqual(bottom.Counts, expected) {
		t.Errorf("Expected counts %v, got %v", expected, bottom.Counts)
	}
}
