package p2p

import (
	"log"
	"time"
)

var maxAttemptsForConnectionRetry = 5
var delayForConnectionRetry = 1 * time.Second
var backoffForConnectionRetry = 2.0

type Node struct {
	Port       string
	Sentence   string
	Delay      time.Duration
	TargetAddr string
}

func (n *Node) Start() error {
	server, err := NewServer(n.Port)

	if err != nil {
		log.Printf("[ERROR] Failed to start node on port %s: %v\n", n.Port, err)
		return err
	}

	server.Start()

	if n.TargetAddr != "" {
		go n.sentenceSender()
	}

	for conn := range server.GetConnections() {
		go HandleIncomingMessages(conn, n.Port)
	}
	return nil
}

func (n *Node) sentenceSender() {

	client := NewClient(n.TargetAddr)

	var conn *Connection

	err := Retry(RetryConfig{
		MaxAttempts: maxAttemptsForConnectionRetry,
		Delay:       delayForConnectionRetry,
		Backoff:     backoffForConnectionRetry,
	}, func() error {
		var connErr error
		conn, connErr = client.Connect()
		return connErr
	})

	if err != nil {
		log.Printf("[ERROR] Failed to connect to %s: %v\n", n.TargetAddr, err)
		return
	}

	defer conn.Close()

	for {
		msg := NewMessage(n.Port, n.Sentence)
		err := SendMessage(conn, msg)
		if err != nil {
			log.Printf("[ERROR] Failed to send sentence: %v\n", err)
		}
		time.Sleep(n.Delay * time.Millisecond)
	}
}
