package suffren

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/florian-roos/suffren/internal/crdt"
	latticeagreement "github.com/florian-roos/suffren/internal/lattice-agreement"
	"github.com/florian-roos/suffren/internal/node"
	"github.com/florian-roos/suffren/internal/p2p"
	"github.com/florian-roos/suffren/pkg/config"
)

// pendingOp tracks an in-flight operation (IncrementKey or ValueForKey) so that
// the learn callback can signal exactly the caller that initiated it.
type pendingOp struct {
	proposedValue crdt.Lattice
	done          chan *crdt.CounterMap // buffered(1) so learn callback never blocks
}

type Suffren struct {
	mu            sync.Mutex // protects localCounters and pending
	node          *node.Node
	localCounters *crdt.CounterMap
	opID          uint64 // ID associated to an operation in the pending map
	pending       map[uint64]*pendingOp
	la            *latticeagreement.LatticeAgreement
	cfg           *config.Config
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, config *config.Config) *Suffren {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	suffren := &Suffren{
		localCounters: crdt.NewCounterMap(nodeIds),
		opID:          0,
		cfg:           config,
	}
	network := p2p.NewNetwork(port, peers)

	suffren.la = latticeagreement.NewLatticeAgreement(
		nodeId,
		peers,
		network,
		suffren.localCounters.Bottom(),
		suffren.onLearn(),
		&config.LatticeAgreement,
	)

	suffren.node = node.NewNode(nodeId, port, peers, network, suffren.la, config)

	return suffren
}

func (s *Suffren) Start() error {
	if err := s.node.Start(); err != nil {
		return err
	}

	// Retry sync() until it succeeds (peers may still be reconnecting).
	deadline := time.Now().Add(s.cfg.Suffren.StartupSyncTimeout)
	for time.Now().Before(deadline) {
		ok := s.sync()
		if ok {
			slog.Info("Startup sync complete")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("startup sync failed: unable to sync with cluster within %v", s.cfg.Suffren.StartupSyncTimeout)
}

// IncrementKey proposes the new value to the cluster and modifiy the local value when the new value is accepted.
// Blocks until a quorum LEARN containing the increment is received or timeout.
func (s *Suffren) IncrementKey(key string, value uint64) (uint64, bool) {
	s.mu.Lock()
	proposed := s.localCounters.Copy()
	proposed.IncrementKey(key, s.node.Id, value)

	opID, done := s.registerPendingLocked(proposed)
	defer s.unregisterPending(opID)

	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, learned := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)
	if !ok {
		return 0, false
	}
	defer s.applyLearned(learned)
	return learned.ValueForKey(key), true
}

// Value performs a linearisable quorum read: proposes the current state,
// waits for LEARN, and returns the value that was committed.
func (s *Suffren) ValueForKey(key string) (uint64, bool) {
	s.mu.Lock()
	proposed := s.localCounters.Copy()

	opID, done := s.registerPendingLocked(proposed)
	defer s.unregisterPending(opID)

	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, learned := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)
	if !ok {
		return 0, false
	}
	defer s.applyLearned(learned)
	return learned.ValueForKey(key), true
}

// Stop the node and its network service gracefully.
func (s *Suffren) Stop() {
	s.node.Stop()
}

// Helpers

func (s *Suffren) waitForLearn(done <-chan *crdt.CounterMap, timeout time.Duration) (bool, *crdt.CounterMap) {
	select {
	case <-time.After(timeout):
		return false, nil
	case value := <-done:
		return true, value
	}
}

// sync() synchronizes the node with a quorum by proposing its local value. If it gets a response of a quorum,
// it returns true and change the localCounters. Else it return false.
func (s *Suffren) sync() bool {
	s.mu.Lock()

	done := make(chan *crdt.CounterMap, 1)
	proposed := s.localCounters.Copy()
	op := &pendingOp{
		proposedValue: proposed,
		done:          done,
	}

	s.pending[s.opID] = op
	s.opID++

	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, value := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)
	if ok {
		s.mu.Lock()
		s.localCounters = value
		s.mu.Unlock()
	}

	return ok
}

// Suffren merges the received value and validates the end of its operation (Value or Increment) only if the value it
// received contain what the suffren node proposed
func (s *Suffren) onLearn() func(crdt.Lattice) {
	return func(learnedValue crdt.Lattice) {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Always merge: monotonic join keeps the local counter up to date.
		s.localCounters.MergeInPlace(learnedValue)

		// Signal the pending operation only if the learned value contains
		// what was proposed (linearisable guarantee for that caller).
		for opID, op := range s.pending {
			if op.proposedValue.IsIn(learnedValue) {
				select {
				case op.done <- s.localCounters.Copy():
					delete(s.pending, opID)
				default: // already signalled or caller timed out
				}
			}
		}
	}
}

// registerPendingLocked adds the operation to the pending map. It returns the opID and the operation termination channel.
// It needs a mutex on s.
func (s *Suffren) registerPendingLocked(proposed crdt.Lattice) (uint64, chan *crdt.CounterMap) {
	done := make(chan *crdt.CounterMap, 1)

	opID := s.opID
	s.pending[opID] = &pendingOp{proposedValue: proposed, done: done}
	s.opID++

	return opID, done
}

// unregisterPending clean the pending map of the operation.
func (s *Suffren) unregisterPending(opID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, opID)
}

// applyLearned updates the localCounters with the learned value.
func (s *Suffren) applyLearned(learned crdt.Lattice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localCounters.MergeInPlace(learned)
}
