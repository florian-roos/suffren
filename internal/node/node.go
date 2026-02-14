package node

import (
	"flatstate/internal/protocol"
	"flatstate/internal/transport"
)

type NetworkService interface {
	Listen() (<-chan transport.IncomingMessage, error) //Start the listening on the server
	Send(addr string, msg protocol.Message)
	Close() error //Close the network service
}

type Store interface {
	Get(key string) (string, bool)
	Put(key string, value string)
}

type MessageHandler interface {
	handleIncomingMessage(msg transport.IncomingMessage, store Store)
}

type Node struct {
	Port       string
	Command    protocol.Command
	TargetAddr string
}
