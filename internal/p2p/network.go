package p2p

import (
	"fmt"
	"log"

	"suffren/internal/crdt"
	"suffren/internal/protocol"
)

type Network struct {
	port   string
	server *Server
	client *Client
	peers  map[crdt.NodeId]string
}

func NewNetwork(port string, peers map[crdt.NodeId]string) *Network {
	return &Network{
		port:   port,
		server: NewServer(port),
		client: NewClient(),
		peers:  peers,
	}
}

func (n *Network) Listen() (<-chan protocol.Message, error) {
	msgChan, err := n.server.Listen()
	if err != nil {
		return nil, fmt.Errorf("network: failed to start listening: %w", err)
	}
	return msgChan, nil
}

func (n *Network) Send(nodeId crdt.NodeId, msg protocol.Message) error {
	addr, exists := n.peers[nodeId]
	if !exists {
		return fmt.Errorf("network: no address found for node id %s", nodeId)
	}
	err := n.client.Send(addr, msg)

	if err != nil {
		return err
	}

	return nil
}

func (n *Network) Broadcast(msg protocol.Message) error {
	for nodeId := range n.peers {
		n.Send(nodeId, msg)
	}
	return nil
}

// BroadcastToOthers broadcasts the message to all peers except the sender.
func (n *Network) BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error {
	for nodeId := range n.peers {
		if nodeId == senderId {
			continue
		}
		n.Send(nodeId, msg)
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
