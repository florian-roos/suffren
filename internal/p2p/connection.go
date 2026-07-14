package p2p

import (
	"bufio"
	"encoding/gob"
	"net"
	"sync"

	"github.com/florian-roos/suffren/internal/protocol"
)

type Connection struct {
	conn    net.Conn
	writer  *bufio.Writer
	encoder *gob.Encoder
	decoder *gob.Decoder

	writeMu sync.Mutex
}

func NewConnection(conn net.Conn) *Connection {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)
	connection := Connection{
		conn:    conn,
		writer:  writer,
		encoder: gob.NewEncoder(writer),
		decoder: gob.NewDecoder(reader),
	}
	return &connection
}

func (connection *Connection) Send(message protocol.Message) error {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	err := connection.encoder.Encode(message)
	if err == nil {
		err = connection.writer.Flush()
	}
	return err
}

func (connection *Connection) Receive() (protocol.Message, error) {

	var message protocol.Message
	err := connection.decoder.Decode(&message)
	return message, err
}

func (c *Connection) Close() error {
	return c.conn.Close()
}
