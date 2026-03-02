package p2p

import (
	"log"
	"net"
	"suffren/internal/protocol"
	"sync"
)

type Client struct {
	pool      map[string]*Connection
	knownDown map[string]struct{} // peers that failed at least once since last success
	mu        sync.RWMutex
}

func NewClient() *Client {
	return &Client{
		pool:      make(map[string]*Connection),
		knownDown: make(map[string]struct{}),
	}
}

func (c *Client) Send(targetAddr string, msg protocol.Message) error {
	c.mu.RLock()
	conn, exists := c.pool[targetAddr]
	c.mu.RUnlock()

	if !exists {
		var err error
		conn, err = c.connect(targetAddr)
		if err != nil {
			return err
		}
	}

	err := conn.Send(msg)

	if err != nil {
		log.Printf("[ERROR] Failed to send message (Message: %v): %v\n", msg, err)
		c.mu.Lock()
		delete(c.pool, targetAddr)
		c.mu.Unlock()
		return err
	}

	return nil
}

func (c *Client) connect(targetAddr string) (*Connection, error) {
	rawConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		c.mu.Lock()
		if _, alreadyDown := c.knownDown[targetAddr]; !alreadyDown {
			log.Printf("[WARN] %s unreachable — suppressing further warnings until it recovers\n", targetAddr)
			c.knownDown[targetAddr] = struct{}{}
		}
		c.mu.Unlock()
		return nil, err
	}

	conn := NewConnection(rawConn)

	c.mu.Lock()
	if _, wasDown := c.knownDown[targetAddr]; wasDown {
		log.Printf("[INFO] Reconnected to %s\n", targetAddr)
		delete(c.knownDown, targetAddr)
	}
	c.pool[targetAddr] = conn
	c.mu.Unlock()

	return conn, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
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
