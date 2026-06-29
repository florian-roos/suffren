package crdt

import "encoding/gob"

// CounterMap is a CRDT that manages multiple named GCounter instances
// It allows tracking independent counters by key
type CounterMap struct {
	Counters map[string]*GCounter
	nodeIds  []NodeId
}

func NewCounterMap(nodeIds []NodeId) *CounterMap {
	return &CounterMap{
		Counters: make(map[string]*GCounter),
		nodeIds:  nodeIds,
	}
}

// Join returns a new CounterMap = cm ⊔ other (component-wise max for each key).
func (cm *CounterMap) Join(other Lattice) Lattice {
	o := other.(*CounterMap)
	result := &CounterMap{
		Counters: make(map[string]*GCounter, max(len(cm.Counters), len(o.Counters))),
		nodeIds:  cm.nodeIds,
	}

	// Copy counters from cm
	for key, counter := range cm.Counters {
		result.Counters[key] = counter.Copy()
	}

	// Merge counters from other
	for key, counter := range o.Counters {
		if existing, exists := result.Counters[key]; exists {
			existing.MergeInPlace(counter)
		} else {
			result.Counters[key] = counter.Copy()
		}
	}

	return result
}

// MergeInPlace updates cm to be cm ⊔ other.
func (cm *CounterMap) MergeInPlace(other Lattice) {
	o := other.(*CounterMap)
	for key, counter := range o.Counters {
		if existing, exists := cm.Counters[key]; exists {
			existing.MergeInPlace(counter)
		} else {
			cm.Counters[key] = counter.Copy()
		}
	}
}

// IsIn returns true if cm ⊑ other (cm is less than or equal to other)
// meaning ∀k: cm[k] ⊑ other[k]
func (cm *CounterMap) IsIn(other Lattice) bool {
	o := other.(*CounterMap)
	if len(cm.Counters) > len(o.Counters) {
		return false
	}
	for key, counter := range cm.Counters {
		otherCounter, exists := o.Counters[key]
		if !exists || !counter.IsIn(otherCounter){ 
			return false
		}
	}
	return true
}

// StrictlyIsIn returns true if cm ⊏ other (cm is strictly less than other)
func (cm *CounterMap) StrictlyIsIn(other Lattice) bool {
	o := other.(*CounterMap)
	return cm.IsIn(o) && !cm.Equals(o)
}

// Equals returns true if cm and other have the same counters with same values
func (cm *CounterMap) Equals(other Lattice) bool {
	o := other.(*CounterMap)
	if len(cm.Counters) != len(o.Counters) {
		return false
	}
	for key, counter := range cm.Counters {
		otherCounter, exists := o.Counters[key]
		if !exists || !counter.Equals(otherCounter) {
			return false
		}
	}
	return true
}

// Bottom returns a new empty CounterMap with
func (cm *CounterMap) Bottom() Lattice {
	return &CounterMap{
		Counters: make(map[string]*GCounter),
		nodeIds:  cm.nodeIds,
	}
}

// IncrementKey increments the counter for the given key by value for nodeId
func (cm *CounterMap) IncrementKey(key string, nodeId NodeId, value uint64) {
	if _, exists := cm.Counters[key]; !exists {
		cm.Counters[key] = NewGCounter(cm.nodeIds)
	} 
	cm.Counters[key].Increment(nodeId, value)
}

// ValueForKey returns the total value of the counter for the given key
func (cm *CounterMap) ValueForKey(key string) uint64 {
	if counter, exists := cm.Counters[key]; exists {
		return counter.Value()
	}
	return 0
}

// Keys returns all keys in the CounterMap
func (cm *CounterMap) Keys() []string {
	keys := make([]string, 0, len(cm.Counters))
	for key := range cm.Counters {
		keys = append(keys, key)
	}
	return keys
}

// Copy returns a deep copy of cm with no shared memory
func (cm *CounterMap) Copy() *CounterMap {
	result := &CounterMap{
		Counters: make(map[string]*GCounter, len(cm.Counters)),
		nodeIds:  cm.nodeIds,
	}
	for key, counter := range cm.Counters {
		result.Counters[key] = counter.Copy()
	}
	return result
}

func init() {
	// Register CounterMap for gob encoding/decoding in protocol messages
	gob.Register(&CounterMap{})
} 

// func (cm *CounterMap) DeleteKey(key string) {} has to be implemented later with the garbage collector







