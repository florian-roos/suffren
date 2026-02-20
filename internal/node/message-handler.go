package node

import (
	"suffren/internal/transport"
)

type DefaultMessageHandler struct{}

func NewDefaultMessageHandler() *DefaultMessageHandler {
	return &DefaultMessageHandler{}
}

func (h *DefaultMessageHandler) HandleIncomingMessage(msg transport.IncomingMessage) {}
