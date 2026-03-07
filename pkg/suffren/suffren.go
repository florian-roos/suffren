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

// pendingOp tracks the in-flight operation (Increment or Value) so that
// the learn callback can signal exactly the caller that initiated it.
type pendingOp struct {
	proposedValue crdt.Lattice
	done          chan *crdt.GCounter // buffered(1) so learn callback never blocks
}

type Suffren struct {
	opMu         sync.Mutex // serialises Increment()/Value() — at most one in flight
	mu           sync.Mutex // protects localCounter and pending
	node         *node.Node
	localCounter *crdt.GCounter
	pending      *pendingOp // non-nil while an operation is waiting for its LEARN
	la           *latticeagreement.LatticeAgreement
	cfg          *config.Config
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, config *config.Config) *Suffren {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	suffren := &Suffren{
		localCounter: crdt.NewGCounter(nodeIds),
		cfg:          config,
	}
	network := p2p.NewNetwork(port, peers)

	suffren.la = latticeagreement.NewLatticeAgreement(nodeId, peers, network, suffren.localCounter.Bottom(), func(learnedValue crdt.Lattice) {
		suffren.mu.Lock()
		defer suffren.mu.Unlock()

		// Always merge: monotonic join keeps the local counter up to date.
		suffren.localCounter.MergeInPlace(learnedValue)

		// Signal the pending operation only if the learned value contains
		// what was proposed (linearisable guarantee for that caller).
		if suffren.pending != nil && suffren.pending.proposedValue.IsIn(learnedValue) {
			select {
			case suffren.pending.done <- suffren.localCounter.Copy():
			default: // already signalled or caller timed out
			}
		}
	}, &config.LatticeAgreement)

	suffren.node = node.NewNode(nodeId, port, peers, network, suffren.la, config)

	return suffren
}

func (s *Suffren) Start() error {
	return s.node.Start()
}

// Increment increments the local counter and proposes the new value to the cluster.
// Blocks until a quorum LEARN containing the increment is received or timeout.
func (s *Suffren) Increment() (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	done := make(chan *crdt.GCounter, 1)

	s.mu.Lock()
	s.localCounter.Increment(s.node.Id)
	proposed := s.localCounter.Copy()
	s.pending = &pendingOp{proposedValue: proposed, done: done}
	s.la.Proposer.Propose(s.localCounter)
	s.mu.Unlock()

	ok, value := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)

	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()

	if ok {
		return value.Value(), true
	}
	return 0, false
}

// Value performs a linearisable quorum read: proposes the current state,
// waits for LEARN, and returns the value that was committed.
func (s *Suffren) Value() (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	done := make(chan *crdt.GCounter, 1)

	s.mu.Lock()
	proposed := s.localCounter.Copy()
	s.pending = &pendingOp{proposedValue: proposed, done: done}
	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, value := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)

	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()

	if ok {
		return value.Value(), true
	}
	return 0, false
}

// Stop the node and its network service gracefully.
func (s *Suffren) Stop() {
	s.node.Stop()
}

func (s *Suffren) waitForLearn(done <-chan *crdt.GCounter, timeout time.Duration) (bool, *crdt.GCounter) {
	select {
	case <-time.After(timeout):
		return false, nil
	case value := <-done:
		return true, value
	}
}
