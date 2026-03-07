package node

import (
	"log"
	"suffren/internal/crdt"
	latticeagreement "suffren/internal/lattice-agreement"
	"suffren/internal/protocol"
	"suffren/pkg/config"
	"sync"
)

type NetworkService interface {
	Listen() (<-chan protocol.Message, error)            //Start the listening on the server
	Send(nodeId crdt.NodeId, msg protocol.Message) error //Send a message to the target node
	Close() error                                        //Close the network service
}

type Node struct {
	Id       crdt.NodeId
	Port     string
	Network  NetworkService
	peers    map[crdt.NodeId]string
	la       *latticeagreement.LatticeAgreement
	cfg      *config.Config
	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewNode(nodeId crdt.NodeId, port string, peers map[crdt.NodeId]string, network NetworkService, la *latticeagreement.LatticeAgreement, cfg *config.Config) *Node {
	n := Node{
		Id:      nodeId,
		Port:    port,
		Network: network,
		peers:   peers,
		la:      la,
		cfg:     cfg,
		done:    make(chan struct{}),
	}

	return &n
}

func (n *Node) Start() error {
	incomingMsgChan, err := n.Network.Listen()

	if err == nil {
		n.la.Start()
		n.wg.Add(2)
		go n.handleIncomingMsgChannel(incomingMsgChan)
		return nil
	} else {
		log.Printf("[ERROR] Node cannot listen to the network: %v\n", err)
		return err
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

func (n *Node) handleIncomingMsgChannel(incomingMsgChan <-chan protocol.Message) {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			log.Printf("[NODE] Shutting down node id %s\n", n.Id)
			return
		case msg, ok := <-incomingMsgChan:
			if !ok {
				log.Printf("[NODE] Incoming message channel closed for node id %s\n", n.Id)
				return
			}
			n.wg.Add(1)
			go func(m protocol.Message) {
				defer n.wg.Done()
				select {
				case <-n.done:
					log.Printf("[NODE] Stopping message handler for node id %s\n", n.Id)
					return
				default:
					n.la.MessageRouter.HandleIncomingMessage(m)
				}
			}(msg)
		}
	}
}

func (n *Node) Stop() {
	n.stopOnce.Do(func() {
		close(n.done)
	})
	n.wg.Wait()
	n.la.Stop()
	n.Network.Close()
}
