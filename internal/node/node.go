package node

import (
	"log"
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
)

type NetworkService interface {
	Listen() (<-chan protocol.Message, error)            //Start the listening on the server
	Send(nodeId crdt.NodeId, msg protocol.Message) error //Send a message to the target node
	Close() error                                        //Close the network service
}

type MessageHandler interface {
	HandleIncomingMessage(msg protocol.Message)
}

type Node struct {
	Id         crdt.NodeId
	Port       string
	Network    NetworkService
	MsgHandler MessageHandler
	peers      map[crdt.NodeId]string
	done       chan struct{}
	wg         sync.WaitGroup
}

func NewNode(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, network NetworkService, msgHandler MessageHandler) *Node {
	n := Node{
		Id:         nodeId,
		Port:       port,
		Network:    network,
		MsgHandler: msgHandler,
		peers:      peers,
		done:       make(chan struct{}),
	}

	return &n
}

func (n *Node) Start() {
	incomingMsgChan, err := n.Network.Listen()

	if err == nil {
		n.wg.Add(1)
		go n.handleIncomingMsgChannel(incomingMsgChan)
	} else {
		log.Printf("[ERROR] Node cannot listen to the network: %v\n", err)
	}
}

func (n *Node) SendTo(nodeId crdt.NodeId, cmd protocol.Command) error {
	msg := protocol.NewMessage(n.Id, cmd)
	err := n.Network.Send(nodeId, msg)
	if err != nil {
		log.Printf("[ERROR] Node cannot send message to %s: %v\n", nodeId, err)
		return err
	}
	return nil
}

func (n *Node) Broadcast(cmd protocol.Command) error {
	var lastErr error
	for nodeId := range n.peers {
		if nodeId == n.Id {
			continue
		}
		err := n.SendTo(nodeId, cmd)
		if err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (n *Node) handleIncomingMsgChannel(incomingMsgChan <-chan protocol.Message) {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			log.Printf("[NODE] Shutting down node id %s\n", n.Id)
			return
		default:
			msg := <-incomingMsgChan
			n.wg.Add(1)
			go func(m protocol.Message) {
				defer n.wg.Done()
				select {
				case <-n.done:
					log.Printf("[NODE] Stopping message handler for node id %s\n", n.Id)
					return
				default:
					n.MsgHandler.HandleIncomingMessage(m)
				}
			}(msg)
		}
	}
}

func (n *Node) Stop() {
	close(n.done)
	n.wg.Wait()
	n.Network.Close()
}
