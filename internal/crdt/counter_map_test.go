package crdt

import (
	"reflect"
	"testing"
)

func TestCounterMap_ValueForKey_NonExistent(t *testing.T) {
	// GIVEN: an empty CounterMap
	// WHEN: we evaluate ValueForKey on a key that does not exist
	// THEN: it returns 0 without crashing
	
	nodeIds := []NodeId{"node1"}
	cm := NewCounterMap(nodeIds)

	if v := cm.ValueForKey("unknown_key"); v != 0 {
		t.Errorf("Expected 0, got %d", v)
	}
}

func TestCounterMap_IncrementKey(t *testing.T) {
	// GIVEN: a CounterMap with some existing keys at 0 and a non-existent key
	// WHEN: we increment a specific key by a value
	// THEN: only that specific key is incremented, the others remain 0, and a new key is created if it didn't exist

	nodeIds := []NodeId{"node1", "node2"}
	cm := NewCounterMap(nodeIds)
	
	// Pre-populate with 0 values
	cm.Counters["existing_zero"] = NewGCounter(nodeIds)
	
	// Increment
	cm.IncrementKey("existing_zero", "node1", 5)
	cm.IncrementKey("new_key", "node2", 10)

	if v := cm.ValueForKey("existing_zero"); v != 5 {
		t.Errorf("Expected existing_zero to be 5, got %d", v)
	}
	if v := cm.ValueForKey("new_key"); v != 10 {
		t.Errorf("Expected new_key to be 10, got %d", v)
	}
}

func TestCounterMap_Join(t *testing.T) {
	// GIVEN: two CounterMaps with disjoint keys and overlapping keys
	// WHEN: we evaluate cm.Join(other)
	// THEN: it returns the component-wise max. Extra keys in 'other' are added, extra keys in 'cm' are kept, overlapping keys are joined.

	nodeIds := []NodeId{"node1", "node2"}
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("shared_key", "node1", 5)
	cm1.IncrementKey("only_in_1", "node1", 2)

	cm2 := NewCounterMap(nodeIds)
	cm2.IncrementKey("shared_key", "node2", 10)
	cm2.IncrementKey("only_in_2", "node2", 3)

	result := cm1.Join(cm2).(*CounterMap)

	if v := result.ValueForKey("shared_key"); v != 15 {
		t.Errorf("Expected shared_key to be 15, got %d", v)
	}
	if v := result.ValueForKey("only_in_1"); v != 2 {
		t.Errorf("Expected only_in_1 to be 2, got %d", v)
	}
	if v := result.ValueForKey("only_in_2"); v != 3 {
		t.Errorf("Expected only_in_2 to be 3, got %d", v)
	}
}

func TestCounterMap_MergeInPlace(t *testing.T) {
	// GIVEN: two CounterMaps with disjoint keys and overlapping keys
	// WHEN: we evaluate cm.MergeInPlace(other)
	// THEN: cm is mutated and its state equals the result of cm.Join(other)

	nodeIds := []NodeId{"node1", "node2"}
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("shared_key", "node1", 5)
	cm1.IncrementKey("only_in_1", "node1", 2)

	cm2 := NewCounterMap(nodeIds)
	cm2.IncrementKey("shared_key", "node2", 10)
	cm2.IncrementKey("only_in_2", "node2", 3)

	cm1.MergeInPlace(cm2)

	if v := cm1.ValueForKey("shared_key"); v != 15 {
		t.Errorf("Expected shared_key to be 15, got %d", v)
	}
	if v := cm1.ValueForKey("only_in_1"); v != 2 {
		t.Errorf("Expected only_in_1 to be 2, got %d", v)
	}
	if v := cm1.ValueForKey("only_in_2"); v != 3 {
		t.Errorf("Expected only_in_2 to be 3, got %d", v)
	}
}

func TestCounterMap_IsIn(t *testing.T) {
	// GIVEN: multiple pairs of CounterMaps
	// WHEN: we evaluate cm.IsIn(other)
	// THEN: it returns true if other has all keys of cm with values >= cm's values. If cm has an extra key, it returns false.

	nodeIds := []NodeId{"node1"}
	
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("keyA", "node1", 5)

	cm2 := NewCounterMap(nodeIds)
	cm2.IncrementKey("keyA", "node1", 5)
	cm2.IncrementKey("keyB", "node1", 2) // other has extra key

	cm3 := NewCounterMap(nodeIds)
	cm3.IncrementKey("keyA", "node1", 3) // other has lesser value

	cm4 := NewCounterMap(nodeIds)
	cm4.IncrementKey("keyB", "node1", 5) // cm has extra key (keyA is missing in other)

	if !cm1.IsIn(cm2) {
		t.Errorf("Expected cm1 to be in cm2 (other has extra key)")
	}
	if cm1.IsIn(cm3) {
		t.Errorf("Expected cm1 NOT to be in cm3 (other has lesser value)")
	}
	if cm1.IsIn(cm4) {
		t.Errorf("Expected cm1 NOT to be in cm4 (cm has extra key)")
	}
}

func TestCounterMap_StrictlyIsIn(t *testing.T) {
	// GIVEN: pairs of CounterMaps
	// WHEN: we evaluate cm.StrictlyIsIn(other)
	// THEN: it returns true if cm.IsIn(other) is true AND cm is not Equals(other)

	nodeIds := []NodeId{"node1"}
	
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("keyA", "node1", 5)

	cm2 := NewCounterMap(nodeIds)
	cm2.IncrementKey("keyA", "node1", 5) // Equal

	cm3 := NewCounterMap(nodeIds)
	cm3.IncrementKey("keyA", "node1", 10) // Strictly greater

	if cm1.StrictlyIsIn(cm2) {
		t.Errorf("Expected false because cm1 equals cm2")
	}
	if !cm1.StrictlyIsIn(cm3) {
		t.Errorf("Expected true because cm1 is strictly in cm3")
	}
}

func TestCounterMap_Equals(t *testing.T) {
	// GIVEN: pairs of CounterMaps
	// WHEN: we evaluate cm.Equals(other)
	// THEN: it returns false if either has an extra key or different values, and true only if all keys and values match exactly.

	nodeIds := []NodeId{"node1"}
	
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("keyA", "node1", 5)

	cm2 := NewCounterMap(nodeIds)
	cm2.IncrementKey("keyA", "node1", 5)

	cm3 := NewCounterMap(nodeIds)
	cm3.IncrementKey("keyA", "node1", 5)
	cm3.IncrementKey("keyB", "node1", 1)

	cm4 := NewCounterMap(nodeIds)
	cm4.IncrementKey("keyA", "node1", 10)

	if !cm1.Equals(cm2) {
		t.Errorf("Expected cm1 to equal cm2")
	}
	if cm1.Equals(cm3) {
		t.Errorf("Expected cm1 NOT to equal cm3 (cm3 has extra key)")
	}
	if cm3.Equals(cm1) {
		t.Errorf("Expected cm3 NOT to equal cm1 (cm3 has extra key)")
	}
	if cm1.Equals(cm4) {
		t.Errorf("Expected cm1 NOT to equal cm4 (different values)")
	}
}

func TestCounterMap_Bottom(t *testing.T) {
	// GIVEN: a populated CounterMap
	// WHEN: we evaluate cm.Bottom()
	// THEN: it returns an empty CounterMap
	
	nodeIds := []NodeId{"node1"}
	cm := NewCounterMap(nodeIds)
	cm.IncrementKey("keyA", "node1", 5)

	bottom := cm.Bottom().(*CounterMap)
	if len(bottom.Counters) != 0 {
		t.Errorf("Expected bottom to have 0 keys")
	}
}

func TestCounterMap_Copy(t *testing.T) {
	// GIVEN: a populated CounterMap
	// WHEN: we evaluate cm.Copy()
	// THEN: it returns a deep copy where mutating the copy does not affect the original
	
	nodeIds := []NodeId{"node1"}
	cm1 := NewCounterMap(nodeIds)
	cm1.IncrementKey("keyA", "node1", 5)

	cm2 := cm1.Copy()
	cm2.IncrementKey("keyA", "node1", 10)
	cm2.IncrementKey("keyB", "node1", 2)

	if v := cm1.ValueForKey("keyA"); v != 5 {
		t.Errorf("Original map was mutated! keyA is %d, expected 5", v)
	}
	if v := cm1.ValueForKey("keyB"); v != 0 {
		t.Errorf("Original map was mutated! keyB is %d, expected 0", v)
	}
	if !reflect.DeepEqual(cm1.nodeIds, cm2.nodeIds) {
		t.Errorf("NodeIds were not copied correctly")
	}
}
