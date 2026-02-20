package p2p

import (
	"suffren/internal/protocol"
	"testing"
	"time"
)

// TestSimpleConnectionReuse tests basic connection reuse
func TestSimpleConnectionReuse(t *testing.T) {
	// Start server
	server := NewServer("9999")
	msgChan, err := server.Listen()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Auto-reply with ACKs
	go func() {
		for msg := range msgChan {
			t.Logf("Server received message: %s", msg.Message.Id)
			ack := protocol.NewAck(msg.Message)
			if err := msg.Reply(ack); err != nil {
				t.Logf("Failed to send ACK: %v", err)
			} else {
				t.Logf("Server sent ACK for: %s", msg.Message.Id)
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)

	// Create client
	client := NewClient()
	defer client.Close()

	// Send first message
	t.Log("Sending first message...")
	msg1 := protocol.NewMessage("test", protocol.Command{Type: protocol.CmdGet})
	if err := client.Send("localhost:9999", msg1); err != nil {
		t.Fatalf("First send failed: %v", err)
	}
	t.Log("First message sent successfully")

	// Small delay
	time.Sleep(100 * time.Millisecond)

	// Send second message (should reuse connection)
	t.Log("Sending second message...")
	msg2 := protocol.NewMessage("test", protocol.Command{Type: protocol.CmdPut, Key: "k"})
	if err := client.Send("localhost:9999", msg2); err != nil {
		t.Fatalf("Second send failed: %v", err)
	}
	t.Log("Second message sent successfully")

	// Check pool size
	client.mu.RLock()
	poolSize := len(client.pool)
	client.mu.RUnlock()

	if poolSize != 1 {
		t.Errorf("Expected pool size 1, got %d", poolSize)
	}

	t.Log("Test completed successfully!")
}
