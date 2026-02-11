package p2p

import (
	"log"
	"net"
)

const connChannelSize = 100

type Server struct {
	port        string
	listener    net.Listener
	connections chan *Connection
}

func NewServer(port string) (*Server, error) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("[ERROR] Failed to create server: %v\n", err)
		return nil, err
	}
	s := Server{
		port:        port,
		listener:    listener,
		connections: make(chan *Connection, connChannelSize),
	}
	return &s, nil
}

func (s Server) Start() {
	go func() {
		log.Printf("[SERVER] Listening on port %s\n", s.port)
		for {
			conn, err := s.accept()
			if err != nil {
				log.Printf("[ERROR] Failed to accept connection%v\n", err)
				continue
			}
			s.connections <- NewConnection(conn)
		}
	}()
}

func (s Server) accept() (net.Conn, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, err
	}
	return conn, err
}

func (s Server) Close() error {
	close(s.connections)
	return s.listener.Close()
}

func (s Server) GetConnections() <-chan *Connection {
	return s.connections
}
