package p2p

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/florian-roos/suffren/internal/protocol"
)

const MsgChanSize = 100

type Server struct {
	address           string
	listener          net.Listener
	done              chan struct{}
	wg                sync.WaitGroup
	activeConnections map[*Connection]struct{}
	mu                sync.Mutex
}

func NewServer(address string) *Server {
	s := &Server{
		address:           address,
		done:              make(chan struct{}),
		wg:                sync.WaitGroup{},
		activeConnections: make(map[*Connection]struct{}),
		mu:                sync.Mutex{},
	}
	return s
}

func (s *Server) Listen() (<-chan protocol.Message, error) {
	var err error

	// Extract the port from the address to bind to all interfaces
	_, port, err := net.SplitHostPort(s.address)
	if err != nil {
		return nil, err
	}
	bindAddress := "[::]:" + port

	s.listener, err = net.Listen("tcp", bindAddress)

	if err != nil {
		slog.Error("Failed to start server", "error", err)
		return nil, err
	}

	slog.Info("Listening", "address", bindAddress, "advertised", s.address)
	msgChanel := make(chan protocol.Message, MsgChanSize)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				if isClosingNetwork(err) {
					slog.Info("Shutting down server", "address", s.address)
					return
				}
				slog.Error("Failed to accept connection", "error", err)
				continue
			}
			s.wg.Add(1)
			go s.handleConnection(NewConnection(conn), msgChanel)
		}

	}()
	return msgChanel, nil
}

func (s *Server) handleConnection(conn *Connection, msgChanel chan protocol.Message) {

	defer s.wg.Done()
	defer func() {
		err := conn.Close()
		if err != nil {
			slog.Debug("Error closing connection", "error", err)
		}
	}()

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
			if errors.Is(err, io.EOF) {
				// Peer closed the connection cleanly — not an error.
				return
			}
			if !isClosingNetwork(err) {
				slog.Warn("Lost connection to peer", "error", err)
				return
			}
		}

		// Check if server is shutting down
		select {
		case <-s.done:
			return
		default:
		}

		msgChanel <- msg
	}
}

func isClosingNetwork(err error) bool {
	return err != nil && strings.Contains(err.Error(), "use of closed network connection")
}

func (s *Server) Close() error {
	slog.Info("Closing server...")
	close(s.done)

	if s.listener != nil {
		err := s.listener.Close()
		if err != nil {
			slog.Error("Closing listener", "error", err)
			return err
		}
	}

	s.mu.Lock()
	for conn := range s.activeConnections {
		err := conn.Close()

		if err != nil {
			slog.Error("Closing connection", "error", err)
			return err
		}
	}

	s.mu.Unlock()

	s.wg.Wait()

	slog.Info("Server closed gracefully")
	return nil
}
