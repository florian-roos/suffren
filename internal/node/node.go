package node

import (
	"flatstate/internal/protocol"
	"flatstate/internal/transport"
	"log"
	"sync"
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
	HandleIncomingMessage(msg transport.IncomingMessage, store *Store)
}

type Node struct {
	Port       string
	Command    protocol.Command
	TargetAddr string
	Network    NetworkService
	Store      Store
	MsgHandler MessageHandler
	done       chan struct{}
	wg         sync.WaitGroup
}

func NewNode(port string, cmd protocol.Command, targetAddr string, network NetworkService, store Store, msgHandler MessageHandler) *Node {
	n := Node{
		Port:       port,
		Command:    cmd,
		TargetAddr: targetAddr,
		Network:    network,
		Store:      store,
		MsgHandler: msgHandler,
		done:       make(chan struct{}),
	}

	go n.start()

	return &n
}

func (n *Node) start() {

	incomingMsgChan, err := n.Network.Listen()

	if err == nil {
		n.wg.Add(1)
		go func() {
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
							n.MsgHandler.HandleIncomingMessage(m, &n.Store)
						}
					}(msg)
				}
			}
		}()
	} else {
		log.Printf("[ERROR] Node cannot listen to the network: %v\n", err)
	}

	n.Network.Send(n.TargetAddr, protocol.NewMessage(n.Port, n.Command))
}

func (n *Node) Stop() {
	close(n.done)
	n.wg.Wait()
	n.Network.Close()
}
