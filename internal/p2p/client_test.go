package p2p

import (
	"flatstate/internal/protocol"
	"flatstate/internal/transport"
	"sync"
	"testing"
	"time"
)

// TestNewClient tests that a new client is properly initialized
func TestNewClient(t *testing.T) {
	client := NewClient()

	if client.pool == nil {
		t.Error("Client pool should be initialized")
	}

	if len(client.pool) != 0 {
		t.Errorf("Client pool should be empty initially, got %d connections", len(client.pool))
	}
}

// TestClient_SendWithPoolReuse tests that the client reuses connections from the pool
func TestClient_SendWithPoolReuse(t *testing.T) {
	// Start a mock server
	server := NewServer("9010")
	msgChan, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Handle incoming messages and send ACKs
	go func() {
		for msg := range msgChan {
			ack := protocol.NewAck(msg.Message)
			if err := msg.Reply(ack); err != nil {
				t.Logf("Failed to send ACK: %v", err)
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Create client
	client := NewClient()
	defer client.Close()

	targetAddr := "localhost:9010"

	// First send - should create a new connection
	msg1 := protocol.NewMessage("test-client", protocol.Command{
		Type:  protocol.CmdPut,
		Key:   "key1",
		Value: "value1",
	})

	if err := client.Send(targetAddr, msg1); err != nil {
		t.Fatalf("First send failed: %v", err)
	}

	// Check pool has 1 connection
	client.mu.RLock()
	poolSize := len(client.pool)
	client.mu.RUnlock()

	if poolSize != 1 {
		t.Errorf("Expected pool size 1 after first send, got %d", poolSize)
	}

	// Second send to same address - should REUSE connection
	msg2 := protocol.NewMessage("test-client", protocol.Command{
		Type: protocol.CmdGet,
		Key:  "key1",
	})

	if err := client.Send(targetAddr, msg2); err != nil {
		t.Fatalf("Second send failed: %v", err)
	}

	// Check pool still has 1 connection (reused)
	client.mu.RLock()
	poolSize = len(client.pool)
	client.mu.RUnlock()

	if poolSize != 1 {
		t.Errorf("Expected pool size 1 after reuse, got %d (connection was not reused!)", poolSize)
	}
}

// TestClient_SendToMultipleAddresses tests that the client maintains separate connections
func TestClient_SendToMultipleAddresses(t *testing.T) {
	// Start 2 servers
	server1 := NewServer("9011")
	msgChan1, err := server1.Listen()
	if err != nil {
		t.Fatalf("Failed to start server1: %v", err)
	}
	defer server1.Close()

	server2 := NewServer("9012")
	msgChan2, err := server2.Listen()
	if err != nil {
		t.Fatalf("Failed to start server2: %v", err)
	}
	defer server2.Close()

	// Handle ACKs for both servers
	go handleACKs(msgChan1)
	go handleACKs(msgChan2)

	time.Sleep(100 * time.Millisecond)

	// Create client
	client := NewClient()
	defer client.Close()

	// Send to first server
	msg1 := protocol.NewMessage("test-client", protocol.Command{
		Type: protocol.CmdPut,
		Key:  "key1",
	})
	if err := client.Send("localhost:9011", msg1); err != nil {
		t.Fatalf("Send to server1 failed: %v", err)
	}

	// Send to second server
	msg2 := protocol.NewMessage("test-client", protocol.Command{
		Type: protocol.CmdPut,
		Key:  "key2",
	})
	if err := client.Send("localhost:9012", msg2); err != nil {
		t.Fatalf("Send to server2 failed: %v", err)
	}

	// Check pool has 2 connections (one per server)
	client.mu.RLock()
	poolSize := len(client.pool)
	client.mu.RUnlock()

	if poolSize != 2 {
		t.Errorf("Expected pool size 2 (one per server), got %d", poolSize)
	}
}

// TestClient_Close tests that Close properly cleans up all connections
func TestClient_Close(t *testing.T) {
	// Start 2 servers
	server1 := NewServer("9013")
	msgChan1, _ := server1.Listen()
	defer server1.Close()

	server2 := NewServer("9014")
	msgChan2, _ := server2.Listen()
	defer server2.Close()

	go handleACKs(msgChan1)
	go handleACKs(msgChan2)

	time.Sleep(100 * time.Millisecond)

	// Create client and send to both servers
	client := NewClient()

	msg := protocol.NewMessage("test-client", protocol.Command{Type: protocol.CmdGet})
	client.Send("localhost:9013", msg)
	client.Send("localhost:9014", msg)

	// Verify connections exist
	client.mu.RLock()
	poolSize := len(client.pool)
	client.mu.RUnlock()

	if poolSize != 2 {
		t.Errorf("Expected 2 connections before Close, got %d", poolSize)
	}

	// Close client
	if err := client.Close(); err != nil {
		t.Errorf("Client.Close() returned error: %v", err)
	}

	// Verify pool is empty
	client.mu.RLock()
	poolSize = len(client.pool)
	client.mu.RUnlock()

	if poolSize != 0 {
		t.Errorf("Expected empty pool after Close, got %d connections", poolSize)
	}
}

// TestClient_ConcurrentSends tests that concurrent sends are thread-safe
func TestClient_ConcurrentSends(t *testing.T) {
	server := NewServer("9015")
	msgChan, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	go handleACKs(msgChan)

	time.Sleep(100 * time.Millisecond)

	client := NewClient()
	defer client.Close()

	targetAddr := "localhost:9015"
	numGoroutines := 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Send messages concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			msg := protocol.NewMessage("test-client", protocol.Command{
				Type:  protocol.CmdPut,
				Key:   "key",
				Value: "value",
			})

			if err := client.Send(targetAddr, msg); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent send failed: %v", err)
	}

	// Pool should still have only 1 connection (all reused)
	client.mu.RLock()
	poolSize := len(client.pool)
	client.mu.RUnlock()

	if poolSize != 1 {
		t.Errorf("Expected pool size 1 after concurrent sends, got %d", poolSize)
	}
}

// Helper function to handle ACKs
func handleACKs(msgChan <-chan transport.IncomingMessage) {
	for msg := range msgChan {
		ack := protocol.NewAck(msg.Message)
		msg.Reply(ack)
	}
}
