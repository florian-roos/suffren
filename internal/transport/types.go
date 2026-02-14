package transport

import "flatstate/internal/protocol"

type IncomingMessage struct {
	Message protocol.Message
	Reply   func(protocol.Message) error
}
