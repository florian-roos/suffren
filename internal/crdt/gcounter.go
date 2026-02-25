package crdt

type NodeId string

type GCounter struct {
	Counts map[NodeId]uint64
}

func NewGCounter(nodeIds []NodeId) *GCounter {
	counts := make(map[NodeId]uint64)
	for _, nodeId := range nodeIds {
		counts[nodeId] = 0
	}
	return &GCounter{Counts: counts}
}

func (g *GCounter) Increment(nodeId NodeId) {
	g.Counts[nodeId]++
}

func (g *GCounter) Value() uint64 {
	var total uint64
	for _, count := range g.Counts {
		total += count
	}
	return total
}

// Join returns a new GCounter = g ⊔ other (component-wise max).
func (g *GCounter) Join(other Lattice) Lattice {
	o := other.(*GCounter)
	result := NewGCounter([]NodeId{})
	for nodeId, count := range g.Counts {
		result.Counts[nodeId] = count
	}
	for nodeId, count := range o.Counts {
		if count > result.Counts[nodeId] {
			result.Counts[nodeId] = count
		}
	}
	return result
}

// IsIn returns true if g ⊑ other (g is less than or equal to other)
// meaning ∀k: g[k] ≤ other[k]
func (g *GCounter) IsIn(other Lattice) bool {
	o := other.(*GCounter)
	for nodeId, count := range g.Counts {
		if count > o.Counts[nodeId] {
			return false
		}
	}
	return true
}

func (g *GCounter) Bottom() Lattice {
	nodeIds := make([]NodeId, 0, len(g.Counts))
	for nodeId := range g.Counts {
		nodeIds = append(nodeIds, nodeId)
	}
	return NewGCounter(nodeIds)
}
