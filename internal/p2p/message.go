package p2p

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	Id        string
	Timestamp time.Time
	Sender    string
	Payload   string
}

func NewMessage(sender string, payload string) Message {
	return Message{
		Id:        uuid.New().String(),
		Timestamp: time.Now(),
		Sender:    sender,
		Payload:   payload,
	}
}
