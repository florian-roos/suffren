package p2p

import (
	"suffren/internal/protocol"
	"suffren/internal/transport"
	"testing"
	"time"
)

// TestNetwork_Creation tests that a Network is properly initialized
func TestNetwork_Creation(t *testing.T) {
	network := NewNetwork("9020")

	if network.server == nil {
		t.Error("Network.server should be initialized")
	}

	if network.client == nil {
		t.Error("Network.client should be initialized")
	}

	if network.port != "9020" {
		t.Errorf("Expected port 9020, got %s", network.port)
	}
}

// TestNetwork_ListenAndSend tests the full cycle: Listen + Send + Receive
func TestNetwork_ListenAndSend(t *testing.T) {
	// Create two networks (simulating two nodes)
	network1 := NewNetwork("9021")
	network2 := NewNetwork("9022")

	// Start listening on both
	_, err := network1.Listen()
	if err != nil {
		t.Fatalf("Failed to start network1: %v", err)
	}
	defer network1.Close()

	msgChan2, err := network2.Listen()
	if err != nil {
		t.Fatalf("Failed to start network2: %v", err)
	}
	defer network2.Close()

	time.Sleep(100 * time.Millisecond)

	// Message from network1 to network2
	testMsg := protocol.NewMessage("network1", protocol.Command{
		Type:  protocol.CmdPut,
		Key:   "test-key",
		Value: "test-value",
	})

	// Send from network1 to network2 (in background - Send waits for ACK)
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- network1.Send("localhost:9022", testMsg)
	}()

	// Network2 should receive the message
	select {
	case incoming := <-msgChan2:
		if incoming.Message.Id != testMsg.Id {
			t.Errorf("Expected message ID %s, got %s", testMsg.Id, incoming.Message.Id)
		}

		if incoming.Message.Payload.Key != "test-key" {
			t.Errorf("Expected key 'test-key', got '%s'", incoming.Message.Payload.Key)
		}

		// Reply with ACK
		ack := protocol.NewAck(incoming.Message)
		if err := incoming.Reply(ack); err != nil {
			t.Errorf("Failed to send ACK: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message on network2")
	}

	// Wait for Send to complete (it waits for ACK)
	select {
	case err := <-sendDone:
		if err != nil {
			t.Errorf("Send failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not complete in time")
	}
}

// TestNetwork_BidirectionalCommunication tests communication in both directions
func TestNetwork_BidirectionalCommunication(t *testing.T) {
	network1 := NewNetwork("9023")
	network2 := NewNetwork("9024")

	msgChan1, _ := network1.Listen()
	msgChan2, _ := network2.Listen()
	defer network1.Close()
	defer network2.Close()

	time.Sleep(100 * time.Millisecond)

	// Auto-reply with ACKs on both sides
	go autoReply(msgChan1)
	go autoReply(msgChan2)

	// Network1 -> Network2
	msg1to2 := protocol.NewMessage("node1", protocol.Command{
		Type: protocol.CmdPut,
		Key:  "key1",
	})

	if err := network1.Send("localhost:9024", msg1to2); err != nil {
		t.Errorf("Network1 -> Network2 failed: %v", err)
	}

	// Network2 -> Network1
	msg2to1 := protocol.NewMessage("node2", protocol.Command{
		Type: protocol.CmdGet,
		Key:  "key2",
	})

	if err := network2.Send("localhost:9023", msg2to1); err != nil {
		t.Errorf("Network2 -> Network1 failed: %v", err)
	}

	// Both sends succeeded
	t.Log("Bidirectional communication successful")
}

// TestNetwork_ConnectionPooling tests that Network reuses connections via Client pool
func TestNetwork_ConnectionPooling(t *testing.T) {
	network1 := NewNetwork("9025")
	network2 := NewNetwork("9026")

	network1.Listen() // Start listening even though we won't receive on it
	msgChan2, _ := network2.Listen()
	defer network1.Close()
	defer network2.Close()

	time.Sleep(100 * time.Millisecond)

	go autoReply(msgChan2)

	// Send multiple messages from network1 to network2
	for i := 0; i < 5; i++ {
		msg := protocol.NewMessage("node1", protocol.Command{
			Type:  protocol.CmdPut,
			Key:   "key",
			Value: "value",
		})

		if err := network1.Send("localhost:9026", msg); err != nil {
			t.Errorf("Send %d failed: %v", i, err)
		}
	}

	// Check that client has only 1 connection in pool (reused)
	network1.client.mu.RLock()
	poolSize := len(network1.client.pool)
	network1.client.mu.RUnlock()

	if poolSize != 1 {
		t.Errorf("Expected 1 pooled connection after 5 sends, got %d", poolSize)
	}
}

// TestNetwork_Close tests that Close properly shuts down both server and client
func TestNetwork_Close(t *testing.T) {
	network1 := NewNetwork("9027")
	network2 := NewNetwork("9028")

	msgChan2, _ := network2.Listen()
	network1.Listen()
	defer network2.Close()

	time.Sleep(100 * time.Millisecond)

	go autoReply(msgChan2)

	// Establish a connection
	msg := protocol.NewMessage("node1", protocol.Command{Type: protocol.CmdGet})
	network1.Send("localhost:9028", msg)

	// Verify connection exists
	network1.client.mu.RLock()
	hasConnections := len(network1.client.pool) > 0
	network1.client.mu.RUnlock()

	if !hasConnections {
		t.Error("Expected at least one connection before Close")
	}

	// Close network1
	done := make(chan error, 1)
	go func() {
		done <- network1.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not complete in time")
	}

	// Verify connections are closed
	network1.client.mu.RLock()
	poolSize := len(network1.client.pool)
	network1.client.mu.RUnlock()

	if poolSize != 0 {
		t.Errorf("Expected empty pool after Close, got %d connections", poolSize)
	}
}

// TestNetwork_MultipleNodes simulates a 3-node network
func TestNetwork_MultipleNodes(t *testing.T) {
	// Create 3 nodes
	node1 := NewNetwork("9030")
	node2 := NewNetwork("9031")
	node3 := NewNetwork("9032")

	msgChan1, _ := node1.Listen()
	msgChan2, _ := node2.Listen()
	msgChan3, _ := node3.Listen()

	defer node1.Close()
	defer node2.Close()
	defer node3.Close()

	time.Sleep(100 * time.Millisecond)

	// Auto-reply on all nodes
	go autoReply(msgChan1)
	go autoReply(msgChan2)
	go autoReply(msgChan3)

	// Node1 sends to Node2 and Node3
	msg1 := protocol.NewMessage("node1", protocol.Command{Type: protocol.CmdPut, Key: "key1"})
	if err := node1.Send("localhost:9031", msg1); err != nil {
		t.Errorf("Node1 -> Node2 failed: %v", err)
	}

	msg2 := protocol.NewMessage("node1", protocol.Command{Type: protocol.CmdPut, Key: "key2"})
	if err := node1.Send("localhost:9032", msg2); err != nil {
		t.Errorf("Node1 -> Node3 failed: %v", err)
	}

	// Node2 sends to Node3
	msg3 := protocol.NewMessage("node2", protocol.Command{Type: protocol.CmdGet, Key: "key3"})
	if err := node2.Send("localhost:9032", msg3); err != nil {
		t.Errorf("Node2 -> Node3 failed: %v", err)
	}

	// Verify node1 has 2 connections in pool (to node2 and node3)
	node1.client.mu.RLock()
	pool1Size := len(node1.client.pool)
	node1.client.mu.RUnlock()

	if pool1Size != 2 {
		t.Errorf("Node1 should have 2 pooled connections, got %d", pool1Size)
	}

	// Verify node2 has 1 connection in pool (to node3)
	node2.client.mu.RLock()
	pool2Size := len(node2.client.pool)
	node2.client.mu.RUnlock()

	if pool2Size != 1 {
		t.Errorf("Node2 should have 1 pooled connection, got %d", pool2Size)
	}

	t.Log("3-node network test successful")
}

// Helper function to auto-reply with ACKs
func autoReply(msgChan <-chan transport.IncomingMessage) {
	for msg := range msgChan {
		ack := protocol.NewAck(msg.Message)
		msg.Reply(ack)
	}
}
