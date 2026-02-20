package p2p

import (
	"suffren/internal/protocol"
	"suffren/internal/transport"
	"log"
	"net"
	"sync"
)

const connChannelSize = 100
const incomingMsgChanSize = 100

type Server struct {
	port              string
	listener          net.Listener
	connections       chan *Connection
	done              chan struct{}
	wg                sync.WaitGroup
	activeConnections map[*Connection]struct{}
	mu                sync.Mutex
}

func NewServer(port string) *Server {
	s := &Server{
		port:              port,
		connections:       make(chan *Connection, connChannelSize),
		done:              make(chan struct{}),
		wg:                sync.WaitGroup{},
		activeConnections: make(map[*Connection]struct{}),
		mu:                sync.Mutex{},
	}
	return s
}

func (s *Server) Listen() (<-chan transport.IncomingMessage, error) {
	var err error
	s.listener, err = net.Listen("tcp", ":"+s.port)

	if err != nil {
		log.Printf("[ERROR] Failed to start server: %v\n", err)
		return nil, err
	}

	log.Printf("[SERVER] Listening on port %s\n", s.port)
	msgChanel := make(chan transport.IncomingMessage, incomingMsgChanSize)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.done:
				log.Printf("[SERVER] Shutting down server on port %s\n", s.port)
				return
			default:
				conn, err := s.listener.Accept()
				if err != nil {
					log.Printf("[ERROR] Failed to accept connection%v\n", err)
					continue
				}
				s.connections <- NewConnection(conn)
				s.wg.Add(1)
				go s.handleConnection(msgChanel)
			}
		}
	}()
	return msgChanel, nil
}

func (s *Server) handleConnection(msgChanel chan transport.IncomingMessage) {
	conn := <-s.connections

	defer s.wg.Done()
	defer conn.Close()

	s.mu.Lock()
	s.activeConnections[conn] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.activeConnections, conn)
		s.mu.Unlock()
	}()

	// Loop to handle multiple messages on the same connection
	for {
		msg, err := conn.Receive()
		if err != nil {
			log.Printf("[ERROR] Failed to receive message: %v\n", err)
			return
		}

		// Check if server is shutting down
		select {
		case <-s.done:
			return
		default:
		}

		msgChanel <- transport.IncomingMessage{
			Message: msg,
			Reply: func(msg protocol.Message) error {
				return conn.Send(msg)
			},
		}
	}
}

func (s *Server) Close() error {
	log.Println("[SERVER] Closing...")
	close(s.done)

	if s.listener != nil {
		err := s.listener.Close()
		if err != nil {
			log.Printf("[ERROR] Closing listener : %v", err)
			return err
		}
	}

	s.mu.Lock()
	for conn := range s.activeConnections {
		err := conn.Close()

		if err != nil {
			log.Printf("[ERROR] Closing connection : %v", err)
			return err
		}
	}

	s.mu.Unlock()

	close(s.connections)

	s.wg.Wait()

	log.Println("[SERVER] Closed gracefully")
	return nil
}
