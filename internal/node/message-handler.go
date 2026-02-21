package node

import "suffren/internal/protocol"

type DefaultMessageHandler struct{}

func NewDefaultMessageHandler() *DefaultMessageHandler {
	return &DefaultMessageHandler{}
}

func (h *DefaultMessageHandler) HandleIncomingMessage(msg protocol.Message) {}
