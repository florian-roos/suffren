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

// pendingOp tracks the in-flight operation (Increment or Value) so that
// the learn callback can signal exactly the caller that initiated it.
type pendingOp struct {
	proposedValue crdt.Lattice
	done          chan *crdt.CounterMap // buffered(1) so learn callback never blocks
}

type Suffren struct {
	opMu         sync.Mutex // serialises Increment()/Value() — at most one in flight
	mu           sync.Mutex // protects localCounters and pending
	node         *node.Node
	localCounters *crdt.CounterMap
	pending      []*pendingOp 
	la           *latticeagreement.LatticeAgreement
	cfg          *config.Config
}

func NewSuffren(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, config *config.Config) *Suffren {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	suffren := &Suffren{
		localCounters: crdt.NewCounterMap(nodeIds),
		cfg:          config,
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
	if 	err := s.node.Start(); err != nil { return err }
	
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

// Increment increments the local counter by the given value and proposes the new value to the cluster.
// Blocks until a quorum LEARN containing the increment is received or timeout.
func (s *Suffren) IncrementKey(key string, value uint64) (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	done := make(chan *crdt.CounterMap, 1)

	s.mu.Lock()
	proposed := s.localCounters.Copy()
	proposed.IncrementKey(key, s.node.Id, value)
	s.pending = append(s.pending, &pendingOp{proposedValue: proposed, done: done})
	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, learned := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)
	s.mu.Lock()
	// We increment only after learning the value that contains the proposed increment of the key (to avoid the case where the increment is proposed but not included in the learned value, and then we timeout).
	if ok {
		s.localCounters.MergeInPlace(learned)
	}
	s.pending = nil
	s.mu.Unlock()

	if ok {
		return learned.ValueForKey(key), true
	}
	return 0, false
}

// Value performs a linearisable quorum read: proposes the current state,
// waits for LEARN, and returns the value that was committed.
func (s *Suffren) ValueForKey(key string) (uint64, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	done := make(chan *crdt.CounterMap, 1)

	s.mu.Lock()
	proposed := s.localCounters.Copy()
	s.pending = append(s.pending, &pendingOp{proposedValue: proposed, done: done})
	s.la.Proposer.Propose(proposed)
	s.mu.Unlock()

	ok, value := s.waitForLearn(done, s.cfg.Suffren.RoundTimeout)

	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()

	if ok {
		return value.ValueForKey(key), true
	}
	return 0, false
}

// Stop the node and its network service gracefully.
func (s *Suffren) Stop() {
	s.node.Stop()
}

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
		done   		 : done, 		
	}
	s.pending = append(s.pending, op)
	
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
		for _, op := range s.pending {
			if op.proposedValue.IsIn(learnedValue) {
				select {
				case op.done <- s.localCounters.Copy():
				default: // already signalled or caller timed out
				}
			}
		}
	}
}