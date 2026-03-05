package latticeagreement

import (
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"suffren/pkg/config"
)

type Network interface {
	Broadcast(msg protocol.Message) error
	BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error
	Send(nodeId crdt.NodeId, msg protocol.Message) error
}

// LatticeAgreement composes the three roles of the protocol.
type LatticeAgreement struct {
	MessageRouter *MessageRouter
	Proposer      *Proposer
	Acceptor      *Acceptor
	Learner       *Learner
	cfg           *config.LatticeAgreementConfig
}

func NewLatticeAgreement(
	nodeId crdt.NodeId,
	peers map[crdt.NodeId]string,
	network Network,
	bottom crdt.Lattice,
	onLearn func(crdt.Lattice),
	cfg *config.LatticeAgreementConfig,
) *LatticeAgreement {
	la := &LatticeAgreement{
		Proposer: NewProposer(network, nodeId, bottom, peers),
		Acceptor: NewAcceptor(network, bottom, nodeId),
		Learner:  NewLearner(nodeId, network, bottom, onLearn),
		cfg:      cfg,
	}
	la.MessageRouter = NewMessageRouter(la.Proposer, la.Acceptor, la.Learner, la.cfg)

	return la
}

// Start starts the message router.
func (la *LatticeAgreement) Start() {
	la.MessageRouter.Start()
}

// Stop stops the message router and waits for all actor goroutines to exit.
func (la *LatticeAgreement) Stop() {
	la.MessageRouter.Stop()
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
