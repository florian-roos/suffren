package p2p

import (
	"fmt"
	"log/slog"
	"net"
	"github.com/florian-roos/suffren/internal/protocol"
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
		// Connection is stale (peer restarted). Remove it and retry once.
		slog.Warn("stale connection detected, reconnecting to peer", "targetAddr", targetAddr, "error", err)
		c.mu.Lock()
		delete(c.pool, targetAddr)
		c.mu.Unlock()
		closeErr := conn.Close()
		if closeErr != nil {
			slog.Debug("Error closing stale connection", "targetAddr", targetAddr, "error", closeErr)
		}

		// Retry with a fresh connection
		conn, dialErr := c.connect(targetAddr)
		if dialErr != nil {
			return fmt.Errorf("retry dial to %s failed: %w (original: %v)", targetAddr, dialErr, err)
		}
		retryErr := conn.Send(msg)
		if retryErr != nil {
			slog.Error("Retry send also failed", "targetAddr", targetAddr, "error", retryErr)
			c.mu.Lock()
			delete(c.pool, targetAddr)
			c.mu.Unlock()
			return retryErr
		}
		return nil
	}

	return nil
}

func (c *Client) connect(targetAddr string) (*Connection, error) {
	rawConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		c.mu.Lock()
		if _, alreadyDown := c.knownDown[targetAddr]; !alreadyDown {
			slog.Warn("peer unreachable, suppressing connection warnings until recovery", "targetAddr", targetAddr)
			c.knownDown[targetAddr] = struct{}{}
		}
		c.mu.Unlock()
		return nil, err
	}

	conn := NewConnection(rawConn)

	c.mu.Lock()
	if _, wasDown := c.knownDown[targetAddr]; wasDown {
		slog.Info("connection established with peer", "targetAddr", targetAddr)
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
			slog.Error("Failure in closing connection", "addr", addr, "error", err)
			return err
		}
		slog.Info("Closed connection", "addr", addr)
	}

	c.pool = make(map[string]*Connection)

	return nil
}
