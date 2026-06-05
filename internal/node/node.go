package node

import (
	"log/slog"
	"github.com/florian-roos/suffren/internal/crdt"
	latticeagreement "github.com/florian-roos/suffren/internal/lattice-agreement"
	"github.com/florian-roos/suffren/internal/protocol"
	"github.com/florian-roos/suffren/pkg/config"
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
		n.wg.Add(1)
		go n.handleIncomingMsgChannel(incomingMsgChan)
		return nil
	} else {
		slog.Error("Node cannot listen to the network", "error", err)
		return err
	}
}

func (n *Node) SendTo(nodeId crdt.NodeId, cmd protocol.Command) error {
	msg := protocol.NewMessage(n.Id, cmd)
	err := n.Network.Send(nodeId, msg)
	if err != nil {
		slog.Error("Node cannot send message", "to", nodeId, "error", err)
		return err
	}
	return nil
}

func (n *Node) handleIncomingMsgChannel(incomingMsgChan <-chan protocol.Message) {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			slog.Info("Shutting down node", "nodeId", n.Id)
			return
		case msg, ok := <-incomingMsgChan:
			if !ok {
				slog.Info("Incoming message channel closed", "nodeId", n.Id)
				return
			}
			n.wg.Add(1)
			go func(m protocol.Message) {
				defer n.wg.Done()
				select {
				case <-n.done:
					slog.Info("Stopping message handler", "nodeId", n.Id)
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
	err := n.Network.Close()
	if err != nil {
		slog.Error("Failed to close network", "error", err)
	}
}
