package latticeagreement

import (
	"log"
	"suffren/internal/protocol"
	"suffren/pkg/config"
	"sync"
)

type MessageRouter struct {
	proposerMailbox chan protocol.Message
	acceptorMailbox chan protocol.Message
	learnerMailbox  chan protocol.Message
	proposer        *Proposer
	acceptor        *Acceptor
	learner         *Learner
	config          *config.LatticeAgreementConfig
	wg              sync.WaitGroup
	done            chan struct{}
	stopOnce        sync.Once
}

func NewMessageRouter(proposer *Proposer, acceptor *Acceptor, learner *Learner, config *config.LatticeAgreementConfig) *MessageRouter {
	r := &MessageRouter{
		proposerMailbox: make(chan protocol.Message, config.MsgChanSize),
		acceptorMailbox: make(chan protocol.Message, config.MsgChanSize),
		learnerMailbox:  make(chan protocol.Message, config.MsgChanSize),
		proposer:        proposer,
		acceptor:        acceptor,
		learner:         learner,
		config:          config,
		done:            make(chan struct{}),
	}
	return r
}

func (r *MessageRouter) HandleIncomingMessage(msg protocol.Message) {
	var target chan protocol.Message
	switch msg.Payload.Type {
	case protocol.Ack, protocol.Nack:
		target = r.proposerMailbox
	case protocol.Propose:
		target = r.acceptorMailbox
	case protocol.Learn:
		target = r.learnerMailbox
	default:
		log.Printf("[ERROR] Received message with unknown type: %v", msg)
		return
	}
	select {
	case target <- msg:
		//Message successfully routed to the right mailbox.
	default:
		// Mailbox is full, dropping the message to avoid blocking. The periodic proposer will re-converge the cluster, so this is not fatal.
		// log.Printf("[WARN] Mailbox for %v is full, dropping message: %v", msg.Payload.Type, msg)
	}
}

func (r *MessageRouter) runProposer() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			return
		case msg := <-r.proposerMailbox:
			switch msg.Payload.Type {
			case protocol.Ack:
				r.proposer.HandleAck(msg)
			case protocol.Nack:
				r.proposer.HandleNack(msg)
			default:
				//Should never happen since HandleIncomingMessage already filters
				log.Printf("[ERROR] Proposer received unexpected message type: %v", msg)
			}

		}
	}
}

func (r *MessageRouter) runAcceptor() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			return
		case msg := <-r.acceptorMailbox:
			r.acceptor.HandlePropose(msg)
		}
	}
}

func (r *MessageRouter) runLearner() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			return
		case msg := <-r.learnerMailbox:
			r.learner.HandleLearn(msg)
		}
	}
}

func (r *MessageRouter) Start() {
	r.wg.Add(3)
	go r.runProposer()
	go r.runAcceptor()
	go r.runLearner()
}

func (r *MessageRouter) Stop() {
	r.stopOnce.Do(func() {
		close(r.done)
	})
	r.wg.Wait()
}
