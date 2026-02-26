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
	Proposer  *Proposer
	Acceptors *Acceptor
	Learner   *Learner
}

func NewLatticeAgreement(
	nodeId crdt.NodeId,
	peers map[crdt.NodeId]string,
	network Network,
	bottom crdt.Lattice,
	onLearn func(crdt.Lattice),
) *LatticeAgreement {
	return &LatticeAgreement{
		Proposer:  NewProposer(network, nodeId, bottom, peers),
		Acceptors: NewAcceptor(network, bottom, nodeId),
		Learner:   NewLearner(nodeId, network, bottom, onLearn),
	}
}
