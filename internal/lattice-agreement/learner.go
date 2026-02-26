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
	// Only update and re-broadcast if the learned value strictly grows to avoid infinite loops.
	if l.learnedValue.StrictlyIsIn(msg.Payload.Value) {
		l.learnedValue = msg.Payload.Value
		msgToBroadcast := protocol.Message{
			Sender: l.nodeId,
			Payload: protocol.Command{
				Type:      protocol.Learn,
				Value:     l.learnedValue,
				SeqNumber: msg.Payload.SeqNumber,
			},
		}
		go func() {
			err := l.network.BroadcastToOthers(msgToBroadcast, l.nodeId)
			if err != nil {
				// Not fatal: the periodic proposer will re-converge the cluster.
				log.Printf("[Learner:%s] LEARN broadcast failed: %v", l.nodeId, err)
			}
		}()
		if l.onLearn != nil {
			//Notify the application that a new value has been learned.
			go l.onLearn(l.learnedValue)
		}
	}
}
