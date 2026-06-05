package latticeagreement

import (
	"log/slog"
	"sync"

	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/protocol"
)

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

	proposedValue crdt.Lattice
	bufferedValue crdt.Lattice
}

func NewProposer(network Network, nodeId crdt.NodeId, initialValue crdt.Lattice, peers map[crdt.NodeId]string) *Proposer {
	return &Proposer{
		network:       network,
		nodeId:        nodeId,
		peers:         peers,
		quorumSize:    len(peers)/2 + 1,
		quorumReached: false,
		acksReceived:  make([]bool, 0),
		proposedValue: initialValue,
		bufferedValue: initialValue,
	}
}

// Propose starts a new proposal round. Called periodically by the node
// to propagate its local CRDT state to the cluster.
func (p *Proposer) Propose(value crdt.Lattice) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.quorumReached = false
	p.acksReceived = make([]bool, 0)
	p.proposedValue = p.bufferedValue.Join(value)
	p.bufferedValue = p.proposedValue

	msg := protocol.Message{
		Sender: p.nodeId,
		Payload: protocol.Command{
			Type:  protocol.Propose,
			Value: p.proposedValue,
		},
	}
	go func() {
		err := p.network.Broadcast(msg)
		if err != nil {
			slog.Error("PROPOSE broadcast failed", "nodeId", p.nodeId, "error", err)
		}
	}()
}

// HandleAck processes an ACK from an acceptor for the current round.
func (p *Proposer) HandleAck(msg protocol.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if msg.Payload.Value.Equals(p.proposedValue) {
		p.acksReceived = append(p.acksReceived, true)
		p.checkAndHandleQuorum()
	}
}

// HandleNack processes a NACK from an acceptor. The payload contains the
// acceptor's acceptedValue, which we merge into bufferedValue before retrying.
func (p *Proposer) HandleNack(msg protocol.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proposedValue.IsIn(msg.Payload.Value) {
		p.acksReceived = append(p.acksReceived, false)
		p.bufferedValue = p.bufferedValue.Join(msg.Payload.Value)
		p.checkAndHandleQuorum()
	} else {
		slog.Debug("NACK ignored: stale value", "nodeId", p.nodeId, "value", msg.Payload.Value, "current", p.proposedValue)
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
					Type:  protocol.Learn,
					Value: p.proposedValue,
				},
			}
			// Broadcast is blocking I/O — goroutine avoids holding the lock during network calls.
			go func() {
				err := p.network.Broadcast(msg)
				if err != nil {
					// Not fatal: the periodic proposer will re-converge the cluster.
					slog.Error("LEARN broadcast failed", "nodeId", p.nodeId, "error", err)
				}
			}()
		} else {
			// At least one NACK: re-propose with the accumulated bufferedValue.
			// bufferedValue already contains the join of all NACK payloads from HandleNack.
			p.proposedValue = p.bufferedValue
			p.acksReceived = make([]bool, 0)
			p.quorumReached = false

			msg := protocol.Message{Sender: p.nodeId,
				Payload: protocol.Command{
					Type:  protocol.Propose,
					Value: p.proposedValue,
				},
			}
			go func() {
				err := p.network.Broadcast(msg)
				if err != nil {
					// Not fatal: the periodic proposer will re-converge the cluster.
					slog.Error("PROPOSE broadcast failed", "nodeId", p.nodeId, "error", err)
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
