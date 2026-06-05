package protocol

import "github.com/florian-roos/suffren/internal/crdt"

type Message struct {
	Sender  crdt.NodeId
	Payload Command
}

func NewMessage(sender crdt.NodeId, cmd Command) Message {
	return Message{
		Sender:  sender,
		Payload: cmd,
	}
}
