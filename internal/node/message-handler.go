package node

import (
	"flatstate/internal/protocol"
)

type MessageConnection interface {
	Send(protocol.Message) error
	Receive() (protocol.Message, error)
	Close() error
}
