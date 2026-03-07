package latticeagreement

import (
	"log"
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
)

type Learner struct {
	mu sync.Mutex

	nodeId  crdt.NodeId
	network Network
	onLearn func(crdt.Lattice) //Called when a new value is learned.

	learnedValue crdt.Lattice
}

func NewLearner(nodeId crdt.NodeId, network Network, bottom crdt.Lattice, onLearn func(crdt.Lattice)) *Learner {
	return &Learner{
		nodeId:       nodeId,
		network:      network,
		onLearn:      onLearn,
		learnedValue: bottom,
	}
}

func (l *Learner) HandleLearn(msg protocol.Message) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ignore stale LEARNs (incoming value is strictly below our already-learned state).
	if !l.learnedValue.IsIn(msg.Payload.Value) {
		return
	}

	// Re-broadcast only when the value strictly grew, to break gossip loops.
	if l.learnedValue.StrictlyIsIn(msg.Payload.Value) {
		l.learnedValue = msg.Payload.Value
		msgToBroadcast := protocol.Message{
			Sender: l.nodeId,
			Payload: protocol.Command{
				Type:  protocol.Learn,
				Value: l.learnedValue,
			},
		}
		go func() {
			err := l.network.BroadcastToOthers(msgToBroadcast, l.nodeId)
			if err != nil {
				// Not fatal: the periodic proposer will re-converge the cluster.
				log.Printf("[Learner:%s] LEARN broadcast failed: %v", l.nodeId, err)
			}
		}()
	}

	// Always notify the application (even for equal values) so that a
	// pending Value()/Increment() whose proposedValue == learnedValue is unblocked.
	if l.onLearn != nil {
		currentLearned := l.learnedValue
		go l.onLearn(currentLearned)
	}
}
