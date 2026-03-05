package latticeagreement

import (
	"log"
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
)

// Acceptor implements the acceptor role of Lattice Agreement.
// It is safe for concurrent use.
type Acceptor struct {
	mu sync.Mutex

	network Network
	nodeId  crdt.NodeId

	acceptedValue crdt.Lattice
}

func NewAcceptor(network Network, bottom crdt.Lattice, nodeId crdt.NodeId) *Acceptor {
	return &Acceptor{
		network:       network,
		nodeId:        nodeId,
		acceptedValue: bottom,
	}
}

func (a *Acceptor) HandlePropose(msg protocol.Message) {
	if msg.Payload.Value == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acceptedValue.IsIn(msg.Payload.Value) {
		// acceptedValue isIn proposed value -> ACK
		a.acceptedValue = msg.Payload.Value
		replyMsg := protocol.Message{
			Sender: a.nodeId,
			Payload: protocol.Command{
				Type:      protocol.Ack,
				SeqNumber: msg.Payload.SeqNumber,
			},
		}
		go func() {
			err := a.network.Send(msg.Sender, replyMsg)
			if err != nil {
				log.Printf("[ERROR] Failed to send ACK message %v to %s: %v\n", replyMsg, msg.Sender, err)
			}
		}()
	} else {
		// acceptedValue is not in proposed value -> NACK with the JOIN with a more recent value to help the proposer converge.
		a.acceptedValue = a.acceptedValue.Join(msg.Payload.Value)
		replyMsg := protocol.Message{
			Sender: a.nodeId,
			Payload: protocol.Command{
				Type:      protocol.Nack,
				Value:     a.acceptedValue,
				SeqNumber: msg.Payload.SeqNumber,
			},
		}
		go func() {
			err := a.network.Send(msg.Sender, replyMsg)
			if err != nil {
				log.Printf("[ERROR] Failed to send NACK message %v to %s: %v\n", replyMsg, msg.Sender, err)
			}
		}()
	}
}
