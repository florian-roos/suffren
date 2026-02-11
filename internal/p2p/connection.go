package p2p

import (
	"encoding/gob"
	"net"
)

type Connection struct {
	conn    net.Conn
	encoder *gob.Encoder
	decoder *gob.Decoder
}

func NewConnection(conn net.Conn) *Connection {

	connection := Connection{
		conn:    conn,
		encoder: gob.NewEncoder(conn),
		decoder: gob.NewDecoder(conn),
	}
	return &connection
}

func (connection Connection) Send(message Message) error {

	return connection.encoder.Encode(message)
}

func (connection Connection) Receive() (Message, error) {

	var message Message
	err := connection.decoder.Decode(&message)
	return message, err
}

func (c *Connection) Close() error {
	return c.conn.Close()
}
