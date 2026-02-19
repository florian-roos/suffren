package crdt

type NodeId string

type GCounter struct {
	counts map[NodeId]uint64
}

func NewGCounter(nodeIds []NodeId) *GCounter {
	counts := make(map[NodeId]uint64)
	for _, nodeId := range nodeIds {
		counts[nodeId] = 0
	}
	return &GCounter{
		counts: counts,
	}
}

func (g *GCounter) Increment(nodeId NodeId) {
	g.counts[nodeId]++
}

func (g *GCounter) Value() uint64 {
	var total uint64
	for _, count := range g.counts {
		total += count
	}
	return total
}

func (g *GCounter) Join(other *GCounter) {
	for nodeId, count := range other.counts {
		g.counts[nodeId] = max(g.counts[nodeId], count)
	}
}

func (g *GCounter) IsIn(other *GCounter) bool {
	for nodeId, count := range other.counts {
		if count < g.counts[nodeId] {
			return false
		}
	}
	return true
}

func (g *GCounter) Bottom() GCounter {
	nodeIds := make([]NodeId, 0, len(g.counts))
	for nodeId := range g.counts {
		nodeIds = append(nodeIds, nodeId)
	}
	return *NewGCounter(nodeIds)
}
