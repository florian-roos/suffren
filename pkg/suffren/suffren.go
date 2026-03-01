package suffren

import (
	"suffren/internal/crdt"
	latticeagreement "suffren/internal/lattice-agreement"
	"suffren/internal/node"
	"suffren/internal/p2p"
	"sync"
)

type Suffren struct {
	mu           sync.RWMutex
	node         *node.Node
	localCounter *crdt.GCounter
	la           *latticeagreement.LatticeAgreement
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string) (*Suffren, error) {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	suffren := &Suffren{
		mu:           sync.RWMutex{},
		localCounter: crdt.NewGCounter(nodeIds),
	}

	network := p2p.NewNetwork(port, peers)

	suffren.la = latticeagreement.NewLatticeAgreement(nodeId, peers, network, suffren.localCounter.Bottom(), func(learnedValue crdt.Lattice) {
		suffren.mu.Lock()
		defer suffren.mu.Unlock()
		suffren.localCounter.MergeInPlace(learnedValue)
	})

	messageHandler := node.NewLAMessageHandler(suffren.la)
	suffren.node = node.NewNode(nodeId, port, peers, network, messageHandler, suffren.la, suffren.localCounter, node.DefaultConfig())

	err := suffren.node.Start()
	if err != nil {
		return nil, err
	}

	return suffren, nil
}

// Increment increments the local counter and proposes the new value to the cluster.
func (s *Suffren) Increment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localCounter.Increment(s.node.Id)
	s.la.Proposer.Propose(s.localCounter)
}

// Return the current value of the counter. Note that this function is wait-free and the value may be stale if there are ongoing proposals in the cluster.
func (s *Suffren) Value() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localCounter.Value()
}

// Stop the node and its network service gracefully.
func (s *Suffren) Stop() {
	s.node.Stop()
}
