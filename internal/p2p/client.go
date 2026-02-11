package p2p

import (
	"log"
	"net"
)

type Client struct {
	targetAddr string
}

func NewClient(targetAddr string) *Client {

	return &Client{
		targetAddr: targetAddr,
	}
}

func (c *Client) Connect() (*Connection, error) {
	conn, err := net.Dial("tcp", c.targetAddr)
	if err != nil {
		log.Printf("[ERROR] Failed to connect to %s: %v\n", c.targetAddr, err)
		return nil, err
	}
	return NewConnection(conn), nil
}
