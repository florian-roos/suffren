package p2p

import (
	"flatstate/internal/protocol"
	"net"
	"testing"
	"time"
)

// TestServer_ListenAndAccept tests that the server can start listening and accept connections
func TestServer_ListenAndAccept(t *testing.T) {
	server := NewServer("9001")

	// Start listening
	msgChan, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect as a client
	conn, err := net.Dial("tcp", "localhost:9001")
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send a message
	client := NewConnection(conn)
	msg := protocol.NewMessage("test-client", protocol.Command{
		Type:  protocol.CmdPut,
		Key:   "key1",
		Value: "value1",
	})

	if err := client.Send(msg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Receive message on server side
	select {
	case incoming := <-msgChan:
		if incoming.Message.Id != msg.Id {
			t.Errorf("Expected message ID %s, got %s", msg.Id, incoming.Message.Id)
		}

		// Send ACK back
		ack := protocol.NewAck(incoming.Message)
		if err := incoming.Reply(ack); err != nil {
			t.Errorf("Failed to send ACK: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Clean shutdown
	if err := server.Close(); err != nil {
		t.Errorf("Failed to close server: %v", err)
	}
}

// TestServer_MultipleConnections tests handling multiple concurrent connections
func TestServer_MultipleConnections(t *testing.T) {
	server := NewServer("9002")

	msgChan, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create 3 clients concurrently
	numClients := 3
	done := make(chan bool, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			conn, err := net.Dial("tcp", "localhost:9002")
			if err != nil {
				t.Errorf("Client %d: failed to connect: %v", clientID, err)
				done <- false
				return
			}
			defer conn.Close()

			client := NewConnection(conn)
			msg := protocol.NewMessage("test-client", protocol.Command{
				Type: protocol.CmdGet,
				Key:  "test-key",
			})

			if err := client.Send(msg); err != nil {
				t.Errorf("Client %d: failed to send: %v", clientID, err)
				done <- false
				return
			}

			// Wait for ACK
			ack, err := client.Receive()
			if err != nil {
				t.Errorf("Client %d: failed to receive ACK: %v", clientID, err)
				done <- false
				return
			}

			if ack.Payload.Type != protocol.CmdAck {
				t.Errorf("Client %d: expected ACK, got %v", clientID, ack.Payload.Type)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Server handles all messages
	for i := 0; i < numClients; i++ {
		select {
		case incoming := <-msgChan:
			ack := protocol.NewAck(incoming.Message)
			if err := incoming.Reply(ack); err != nil {
				t.Errorf("Failed to send ACK: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Timeout waiting for messages")
		}
	}

	// Wait for all clients to finish
	for i := 0; i < numClients; i++ {
		if success := <-done; !success {
			t.Error("At least one client failed")
		}
	}

	if err := server.Close(); err != nil {
		t.Errorf("Failed to close server: %v", err)
	}
}

// TestServer_CloseGracefully tests that server closes without hanging
func TestServer_CloseGracefully(t *testing.T) {
	server := NewServer("9003")

	_, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create a connection but don't close it yet
	conn, err := net.Dial("tcp", "localhost:9003")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Close server (should close active connections)
	done := make(chan error, 1)
	go func() {
		done <- server.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Server.Close() returned error: %v", err)
		}
		// Success - server closed quickly
	case <-time.After(3 * time.Second):
		t.Fatal("Server.Close() hung - did not complete in time")
	}

	conn.Close()
}
