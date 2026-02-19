package node

import (
	"flatstate/internal/protocol"
	"flatstate/internal/transport"
	"log"
	"sync"
)

type NetworkService interface {
	Listen() (<-chan transport.IncomingMessage, error) //Start the listening on the server
	Send(addr string, msg protocol.Message) error      //Send a message to the target address
	Close() error                                      //Close the network service
}

type MessageHandler interface {
	HandleIncomingMessage(msg transport.IncomingMessage)
}

type Node struct {
	Port       string
	Network    NetworkService
	MsgHandler MessageHandler
	done       chan struct{}
	wg         sync.WaitGroup
}

func NewNode(port string, network NetworkService, msgHandler MessageHandler) *Node {
	n := Node{
		Port:       port,
		Network:    network,
		MsgHandler: msgHandler,
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

func (n *Node) SendCommand(cmd protocol.Command, targetAddr string) error {
	msg := protocol.NewMessage(n.Port, cmd)
	err := n.Network.Send(targetAddr, msg)
	if err != nil {
		log.Printf("[ERROR] Node cannot send message to %s: %v\n", targetAddr, err)
		return err
	}
	return nil
}

func (n *Node) handleIncomingMsgChannel(incomingMsgChan <-chan transport.IncomingMessage) {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			log.Printf("[NODE] Shutting down node on port %s\n", n.Port)
			return
		default:
			msg := <-incomingMsgChan
			n.wg.Add(1)
			go func(m transport.IncomingMessage) {
				defer n.wg.Done()
				select {
				case <-n.done:
					log.Printf("[NODE] Stopping message handler for node on port %s\n", n.Port)
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
