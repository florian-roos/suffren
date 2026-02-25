package latticeagreement

import (
	"log"
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
)

// Network is the subset of the p2p layer the Proposer needs.
type Network interface {
	Broadcast(msg protocol.Message) error
}

// Proposer implements the proposer role of Lattice Agreement.
// It is safe for concurrent use.
type Proposer struct {
	mu sync.Mutex

	network    Network
	nodeId     crdt.NodeId
	peers      map[crdt.NodeId]string
	quorumSize int

	// quorumReached gates checkAndHandleQuorum to fire exactly once per round.
	quorumReached bool
	acksReceived  []bool
	// bufferedValue is the join of all values seen (own proposals + NACK payloads).
	// It is monotonically non-decreasing and is what gets learned.
	bufferedValue crdt.Lattice
	seqNumber     uint64
}

func NewProposer(network Network, nodeId crdt.NodeId, initialValue crdt.Lattice, peers map[crdt.NodeId]string) *Proposer {
	return &Proposer{
		network:       network,
		nodeId:        nodeId,
		peers:         peers,
		quorumSize:    len(peers)/2 + 1,
		quorumReached: false,
		acksReceived:  make([]bool, 0),
		bufferedValue: initialValue,
		seqNumber:     0,
	}
}

// Propose starts a new proposal round. Called periodically by the node
// to propagate its local CRDT state to the cluster.
func (p *Proposer) Propose(value crdt.Lattice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quorumReached = false
	p.seqNumber++
	p.acksReceived = make([]bool, 0)
	p.bufferedValue = p.bufferedValue.Join(value)
	msg := protocol.Message{
		Sender: p.nodeId,
		Payload: protocol.Command{
			Type:      protocol.Propose,
			Lattice:   p.bufferedValue,
			SeqNumber: p.seqNumber,
		},
	}
	go func() {
		err := p.network.Broadcast(msg)
		if err != nil {
			log.Printf("[Proposer:%s] PROPOSE broadcast failed: %v", p.nodeId, err)
		}
	}()
}

// HandleAck processes an ACK from an acceptor for the current round.
func (p *Proposer) HandleAck(msg protocol.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if msg.Payload.SeqNumber == p.seqNumber {
		p.acksReceived = append(p.acksReceived, true)
		p.checkAndHandleQuorum()
	}
}

// HandleNack processes a NACK from an acceptor. The payload contains the
// acceptor's acceptedValue, which we merge into bufferedValue before retrying.
func (p *Proposer) HandleNack(msg protocol.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if msg.Payload.SeqNumber == p.seqNumber {
		p.acksReceived = append(p.acksReceived, false)
		p.bufferedValue = p.bufferedValue.Join(msg.Payload.Lattice)
		p.checkAndHandleQuorum()
	}
}

// checkAndHandleQuorum must be called with p.mu held.
// On a clean quorum (all ACKs), broadcast LEARN.
// On a dirty quorum (any NACK), re-propose with the accumulated bufferedValue.
func (p *Proposer) checkAndHandleQuorum() {
	if p.quorumReached {
		return
	}
	if len(p.acksReceived) >= p.quorumSize {
		p.quorumReached = true
		if noNacks(p.acksReceived) {
			msg := protocol.Message{
				Sender: p.nodeId,
				Payload: protocol.Command{
					Type:      protocol.Learn,
					Lattice:   p.bufferedValue,
					SeqNumber: p.seqNumber,
				},
			}
			// Broadcast is blocking I/O — goroutine avoids holding the lock during network calls.
			go func() {
				err := p.network.Broadcast(msg)
				if err != nil {
					// Not fatal: the periodic proposer will re-converge the cluster.
					log.Printf("[Proposer:%s] LEARN broadcast failed: %v", p.nodeId, err)
				}
			}()
			log.Printf("[Proposer:%s] Quorum reached with all ACKs, broadcasting LEARN for value: %v\n", p.nodeId, p.bufferedValue)
		} else {
			// At least one NACK: re-propose with the accumulated bufferedValue.
			// bufferedValue already contains the join of all NACK payloads from HandleNack.
			p.seqNumber++
			p.acksReceived = make([]bool, 0)
			p.quorumReached = false
			msg := protocol.Message{Sender: p.nodeId,
				Payload: protocol.Command{
					Type:      protocol.Propose,
					Lattice:   p.bufferedValue,
					SeqNumber: p.seqNumber,
				},
			}
			go func() {
				err := p.network.Broadcast(msg)
				if err != nil {
					// Not fatal: the periodic proposer will re-converge the cluster.
					log.Printf("[Proposer:%s] PROPOSE broadcast failed: %v", p.nodeId, err)
				}
			}()
		}
	}
}

func noNacks(acks []bool) bool {
	for _, ack := range acks {
		if !ack {
			return false
		}
	}
	return true
}
