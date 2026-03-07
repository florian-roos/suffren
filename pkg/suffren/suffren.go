package suffren

import (
	"suffren/internal/crdt"
	latticeagreement "suffren/internal/lattice-agreement"
	"suffren/internal/node"
	"suffren/internal/p2p"
	"suffren/pkg/config"
	"sync"
	"time"
)

type Suffren struct {
	opMu                 sync.Mutex // ensure one Increment()/Value() operation at a time
	mu                   sync.RWMutex
	node                 *node.Node
	localCounter         *crdt.GCounter
	ProposedValueChannel chan *crdt.GCounter
	la                   *latticeagreement.LatticeAgreement
	cfg                  *config.Config
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, config *config.Config) *Suffren {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	suffren := &Suffren{
		opMu:                 sync.Mutex{},
		mu:                   sync.RWMutex{},
		localCounter:         crdt.NewGCounter(nodeIds),
		ProposedValueChannel: make(chan *crdt.GCounter, 1),
		cfg:                  config,
	}
	network := p2p.NewNetwork(port, peers)

	suffren.la = latticeagreement.NewLatticeAgreement(nodeId, peers, network, suffren.localCounter.Bottom(), func(learnedValue crdt.Lattice) {
		suffren.mu.Lock()
		defer suffren.mu.Unlock()
		if suffren.localCounter.IsIn(learnedValue) {
			suffren.localCounter.MergeInPlace(learnedValue)
			suffren.ProposedValueChannel <- suffren.localCounter.Copy()
		}
	}, &config.LatticeAgreement)

	suffren.node = node.NewNode(nodeId, port, peers, network, suffren.la, config)

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
func (s *Suffren) Increment() (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	s.localCounter.Increment(s.node.Id)
	s.la.Proposer.Propose(s.localCounter)
	s.mu.Unlock()
	ok, value := s.waitForLearn(s.cfg.Suffren.RoundTimeout)
	if ok {
		return value.Value(), true
	}
	return 0, false
}

// Return the current value of the counter. Note that this function is wait-free and the value may be stale if there are ongoing proposals in the cluster.
func (s *Suffren) Value() (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.RLock()
	s.la.Proposer.Propose(s.localCounter.Copy())
	s.mu.RUnlock()
	ok, value := s.waitForLearn(s.cfg.Suffren.RoundTimeout)
	if ok {
		return value.Value(), true
	}
	return 0, false
}

// Stop the node and its network service gracefully.
func (s *Suffren) Stop() {
	s.node.Stop()
}

func (s *Suffren) waitForLearn(timeout time.Duration) (bool, *crdt.GCounter) {
	select {
	case <-time.After(timeout):
		return false, nil
	case value := <-s.ProposedValueChannel:
		return true, value
	}
}
