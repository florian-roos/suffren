package protocol

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	Id        string
	Timestamp time.Time
	Sender    string
	Payload   Command
}

func NewMessage(sender string, cmd Command) Message {
	return Message{
		Id:        uuid.New().String(),
		Timestamp: time.Now(),
		Sender:    sender,
		Payload:   cmd,
	}
}

func NewAck(receivedMsg Message) Message {
	return Message{
		Id:      receivedMsg.Id,
		Payload: Command{Type: CmdAck},
	}
}

func NewNack(receivedMsg Message) Message {
	return Message{
		Id:      receivedMsg.Id,
		Payload: Command{Type: CmdNack},
	}
}
