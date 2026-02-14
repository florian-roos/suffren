package p2p

import (
	"flatstate/internal/protocol"
	"flatstate/pkg/utils"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const maxAttemptsForConnectionRetry = 5
const delayForConnectionRetry = 1 * time.Second
const backoffForConnectionRetry = 2.0

type Client struct {
	pool map[string]*Connection
	mu   sync.RWMutex
}

func NewClient() *Client {
	return &Client{
		pool: make(map[string]*Connection),
	}
}

func (c *Client) Send(targetAddr string, msg protocol.Message) error {
	c.mu.RLock()
	conn, exists := c.pool[targetAddr]
	c.mu.RUnlock()

	if !exists {
		var err error
		conn, err = c.createConnection(targetAddr)
		if err != nil {
			return err
		}
	}

	err := conn.Send(msg)

	if err != nil {
		log.Printf("[ERROR] Failed to send message (Message.Id: %s): %v\n", msg.Id, err)
		return err
	}

	ackReceived, err := conn.Receive()

	if err != nil {
		log.Printf("[ERROR] Failed to receive ACK for message (Message.Id: %s): %v\n", msg.Id, err)
		return err
	}

	if ackReceived.Payload.Type != protocol.CmdAck || ackReceived.Id == msg.Id {
		log.Printf("[ERROR] Invalid ACK received: %v, %v\n", ackReceived, msg)
		return fmt.Errorf("invalid ACK")
	}

	log.Printf("ACK received for message ID: %s\n", msg.Id)
	return nil
}

func (c *Client) createConnection(targetAddr string) (*Connection, error) {
	var conn net.Conn

	err := utils.Retry(utils.RetryConfig{
		MaxAttempts: maxAttemptsForConnectionRetry,
		Delay:       delayForConnectionRetry,
		Backoff:     backoffForConnectionRetry,
	}, func() error {
		var connErr error
		conn, connErr = net.Dial("tcp", targetAddr)

		if connErr != nil {
			log.Printf("[ERROR] Failed to connect to %s: %v\n", targetAddr, connErr)
			return connErr
		}

		return connErr
	})

	if err != nil {
		return nil, err
	}

	connection := NewConnection(conn)

	c.mu.Lock()
	c.pool[targetAddr] = connection
	c.mu.Unlock()

	return connection, nil
}

func (c *Client) Close() error {
	c.mu.RLock()
	defer c.mu.Unlock()

	for addr, conn := range c.pool {
		err := conn.Close()
		if err != nil {
			log.Printf("[ERROR] Failure in closing connection to %s: %v\n", addr, err)
			return err
		}
		log.Printf("[CLIENT] Closed connection to %s\n", addr)
	}

	c.pool = make(map[string]*Connection)

	return nil
}
