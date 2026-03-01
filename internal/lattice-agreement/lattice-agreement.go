package latticeagreement

import (
	"suffren/internal/crdt"
	"suffren/internal/protocol"
)

type Network interface {
	Broadcast(msg protocol.Message) error
	BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error
	Send(nodeId crdt.NodeId, msg protocol.Message) error
}

// LatticeAgreement composes the three roles of the protocol.
type LatticeAgreement struct {
	Proposer *Proposer
	Acceptor *Acceptor
	Learner  *Learner
}

func NewLatticeAgreement(
	nodeId crdt.NodeId,
	peers map[crdt.NodeId]string,
	network Network,
	bottom crdt.Lattice,
	onLearn func(crdt.Lattice),
) *LatticeAgreement {
	return &LatticeAgreement{
		Proposer: NewProposer(network, nodeId, bottom, peers),
		Acceptor: NewAcceptor(network, bottom, nodeId),
		Learner:  NewLearner(nodeId, network, bottom, onLearn),
	}
}

// Forwarding the methods to implement LAHandler interface
func (la *LatticeAgreement) HandlePropose(msg protocol.Message) {
	la.Acceptor.HandlePropose(msg)
}

func (la *LatticeAgreement) HandleAck(msg protocol.Message) {
	la.Proposer.HandleAck(msg)
}

func (la *LatticeAgreement) HandleNack(msg protocol.Message) {
	la.Proposer.HandleNack(msg)
}

func (la *LatticeAgreement) HandleLearn(msg protocol.Message) {
	la.Learner.HandleLearn(msg)
}
