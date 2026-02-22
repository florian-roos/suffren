package node

import (
	"log"
	"suffren/internal/protocol"
)

type LAHandler interface {
	HandlePropose(msg protocol.Message)
	HandleAck(msg protocol.Message)
	HandleNack(msg protocol.Message)
	HandleLearn(msg protocol.Message)
}

type LAMessageHandler struct {
	la LAHandler
}

func NewLAMessageHandler(la LAHandler) *LAMessageHandler {
	if la == nil {
		panic("LAHandler cannot be nil")
	}
	return &LAMessageHandler{la: la}
}

func (h *LAMessageHandler) HandleIncomingMessage(msg protocol.Message) {
	switch msg.Payload.Type {
	case protocol.Propose:
		h.la.HandlePropose(msg)
	case protocol.Ack:
		h.la.HandleAck(msg)
	case protocol.Nack:
		h.la.HandleNack(msg)
	case protocol.Learn:
		h.la.HandleLearn(msg)
	default:
		log.Printf("[WARN] Unknown command type: %d\n", msg.Payload.Type)
	}
}
