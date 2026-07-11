package engine

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/florian-roos/suffren/internal/config"
	"github.com/florian-roos/suffren/internal/crdt"
	latticeagreement "github.com/florian-roos/suffren/internal/latticeagreement"
	"github.com/florian-roos/suffren/internal/node"
	"github.com/florian-roos/suffren/internal/p2p"
)

// Tracks an in-flight operation (IncrementKey or ValueForKey) so that
// the learn callback can signal exactly the caller that initiated it.
type pendingOp struct {
	proposedValue crdt.Lattice
	done          chan *crdt.CounterMap // buffered(1) so learn callback never blocks
}

type Engine struct {
	mu            sync.Mutex // protects localCounters and pending
	node          *node.Node
	localCounters *crdt.CounterMap
	opID          uint64 // ID associated to an operation in the pending map
	pending       map[uint64]*pendingOp
	unflushedOps  int
	flushTrigger  chan struct{}
	la            *latticeagreement.LatticeAgreement
	cfg           *config.Config
}

func New(nodeId crdt.NodeId, peers map[crdt.NodeId]string, config *config.Config) *Engine {
	var nodeIds []crdt.NodeId
	for nodeId := range peers {
		nodeIds = append(nodeIds, nodeId)
	}

	address := peers[nodeId]

	engine := &Engine{
		localCounters: crdt.NewCounterMap(nodeIds),
		opID:          0,
		pending:       make(map[uint64]*pendingOp),
		unflushedOps:  0,
		flushTrigger:  make(chan struct{}, 1),
		cfg:           config,
	}
	network := p2p.NewNetwork(address, peers)

	engine.la = latticeagreement.NewLatticeAgreement(
		nodeId,
		peers,
		network,
		engine.localCounters.Bottom(),
		engine.onLearn(),
		&config.LatticeAgreement,
	)

	engine.node = node.NewNode(nodeId, address, peers, network, engine.la, config)

	return engine
}

func (s *Engine) Start() error {
	if err := s.node.Start(); err != nil {
		return err
	}

	// Retry sync() until it succeeds (peers may still be reconnecting).
	deadline := time.Now().Add(s.cfg.Engine.StartupSyncTimeout)
	for time.Now().Before(deadline) {
		ok := s.sync()
		if ok {
			slog.Info("Startup sync complete")
			go s.flusher()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("startup sync failed: unable to sync with cluster within %v", s.cfg.Engine.StartupSyncTimeout)
}

// Increments the given key in localCounters and adds the operation to the batch.
// Blocks until a quorum LEARN containing the increment is received or timeout.
func (s *Engine) IncrementKey(key string, value uint64) (uint64, bool) {
	s.mu.Lock()
	s.localCounters.IncrementKey(key, s.node.Id, value)

	opID, done := s.registerPendingLocked(s.localCounters)
	defer s.unregisterPending(opID)

	s.addOpToBatchLocked()
	s.mu.Unlock()

	slog.Debug("Starting IncrementKey operation", "opID", opID, "key", key, "incValue", value)
	start := time.Now()

	ok, learned := s.waitForLearn(done, s.cfg.Engine.RoundTimeout)
	if !ok {
		slog.Error("IncrementKey failed: timeout waiting for learn", "opID", opID)
		return 0, false
	}

	latency := time.Since(start)
	slog.Info("IncrementKey successful (Quorum reached)", "opID", opID, "key", key, "latency_ms", latency.Milliseconds())
	return learned.ValueForKey(key), true
}

// Value performs a linearisable quorum read: proposes the current state,
// waits for LEARN, and returns the value that was committed.
func (s *Engine) ValueForKey(key string) (uint64, bool) {
	s.mu.Lock()
	opID, done := s.registerPendingLocked(s.localCounters)
	defer s.unregisterPending(opID)
	s.addOpToBatchLocked()
	s.mu.Unlock()

	slog.Debug("Starting ValueForKey operation", "opID", opID, "key", key)
	start := time.Now()

	ok, learned := s.waitForLearn(done, s.cfg.Engine.RoundTimeout)
	if !ok {
		slog.Error("ValueForKey failed: timeout waiting for learn", "opID", opID)
		return 0, false
	}

	latency := time.Since(start)
	slog.Info("ValueForKey successful (Quorum reached)", "opID", opID, "key", key, "latency_ms", latency.Milliseconds())
	return learned.ValueForKey(key), true
}

func (s *Engine) ValueForKeyLocal(key string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localCounters.ValueForKey(key)
}

// Stops the node and its network service gracefully.
func (s *Engine) Stop() {
	s.node.Stop()
}

// Runs the flush function when the BatchTimeout timer expired or the MaxBatchSize is over.
func (s *Engine) flusher() {
	ticker := time.NewTicker(s.cfg.Engine.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C: // The BatchTimeout timer expired
			s.flush()
		case <-s.flushTrigger: // We reached the the MaxBatchSize
			s.flush()
		}
	}
}

func (s *Engine) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unflushedOps == 0 {
		return
	}
	s.la.Proposer.Propose(s.localCounters)
	s.unflushedOps = 0

}

// Helpers

func (s *Engine) waitForLearn(done <-chan *crdt.CounterMap, timeout time.Duration) (bool, *crdt.CounterMap) {
	select {
	case <-time.After(timeout):
		slog.Error("Timeout, quorum didn't respond", slog.String("timeout", timeout.String()))
		return false, nil
	case value := <-done:
		return true, value
	}
}

// sync() synchronizes the node with a quorum by proposing its local value. If it gets a response of a quorum,
// it returns true and change the localCounters. Else it return false.
func (s *Engine) sync() bool {
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

	ok, value := s.waitForLearn(done, s.cfg.Engine.RoundTimeout)
	if ok {
		s.mu.Lock()
		s.localCounters = value
		s.mu.Unlock()
	}

	return ok
}

// merges the received value and validates the end of its operation (Value or Increment) only if the value it
// received contain what the node proposed
func (s *Engine) onLearn() func(crdt.Lattice) {
	return func(learnedValue crdt.Lattice) {
		s.mu.Lock()
		defer s.mu.Unlock()

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

// Adds the operation to the pending map. It returns the opID and the operation termination channel.
// It needs a mutex on s.
func (s *Engine) registerPendingLocked(proposed crdt.Lattice) (uint64, chan *crdt.CounterMap) {
	done := make(chan *crdt.CounterMap, 1)

	opID := s.opID
	s.pending[opID] = &pendingOp{proposedValue: proposed, done: done}
	s.opID++

	return opID, done
}

// Cleans the pending map of the operation.
func (s *Engine) unregisterPending(opID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, opID)
}

// Increments unflushedOps and tests whether the maxBatchSize is reached.
// It needs a mutex on s.
func (s *Engine) addOpToBatchLocked() {
	s.unflushedOps++
	if s.unflushedOps >= s.cfg.Engine.MaxBatchSize {
		// We triger the flusher so it flush immediatly
		select {
		case s.flushTrigger <- struct{}{}:
		default:
		}
	}
}
