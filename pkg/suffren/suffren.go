package suffren

import (
	"suffren/internal/crdt"
	latticeagreement "suffren/internal/lattice-agreement"
	"suffren/internal/node"
	"suffren/internal/p2p"
	"suffren/pkg/config"
	"sync"
)

type Suffren struct {
	mu           sync.RWMutex
	node         *node.Node
	localCounter *crdt.GCounter
	la           *latticeagreement.LatticeAgreement
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, config *config.Config) *Suffren {
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
	}, &config.LatticeAgreement)

	suffren.node = node.NewNode(nodeId, port, peers, network, suffren.la, func() crdt.Lattice {
		// A RLock snapshot avoids a data race between periodicPropose (reader)
		// and the learn callback (writer), both of which access localCounter.
		suffren.mu.RLock()
		defer suffren.mu.RUnlock()
		return suffren.localCounter.Copy()
	}, config)

	return suffren
}

func (s *Suffren) Start() error {
	err := s.node.Start()
	if err != nil {
		return err
	}
	return nil
}

// Increment increments the local counter and proposes the new value to the cluster.
func (s *Suffren) Increment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localCounter.Increment(s.node.Id)
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
