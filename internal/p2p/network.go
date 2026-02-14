package p2p

import (
	"fmt"
	"log"

	"flatstate/internal/protocol"
	"flatstate/internal/transport"
)

type Network struct {
	port   string
	server *Server
	client *Client
}

func NewNetwork(port string) *Network {
	return &Network{
		port:   port,
		server: NewServer(port),
		client: NewClient(),
	}
}

func (n *Network) Listen() (<-chan transport.IncomingMessage, error) {
	msgChan, err := n.server.Listen()
	if err != nil {
		return nil, fmt.Errorf("network: failed to start listening: %w", err)
	}
	return msgChan, nil
}

func (n *Network) Send(addr string, msg protocol.Message) error {
	err := n.client.Send(addr, msg)

	if err != nil {
		return err
	}

	return nil
}

func (n *Network) Close() error {
	log.Println("[NETWORK] Closing network service...")

	err := n.server.Close()

	if err != nil {
		log.Printf("[ERROR] Failed to close server: %v\n", err)
		//Continue to close client
	}

	err = n.client.Close()

	if err != nil {
		log.Printf("[ERROR] Failed to close client: %v\n", err)
		return err
	}

	log.Println("[NETWORK] Network service closed gracefully")
	return nil
}
