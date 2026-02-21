package p2p

import (
	"net"
	"strings"
	"testing"
	"time"

	"suffren/internal/crdt"
	"suffren/internal/protocol"
)

// helper: get a free port from the OS
func getFreePort() string {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	port := strings.Split(listener.Addr().String(), ":")
	return port[len(port)-1] // returns "36277" instead of "[::]:36277"
}

func TestSendReceiveReply(t *testing.T) {
	port1 := getFreePort()
	port2 := getFreePort()
	network1 := NewNetwork(port1, map[crdt.NodeId]string{
		crdt.NodeId("node1"): "localhost:" + port1,
		crdt.NodeId("node2"): "localhost:" + port2,
	})
	network2 := NewNetwork(port2, map[crdt.NodeId]string{
		crdt.NodeId("node1"): "localhost:" + port1,
		crdt.NodeId("node2"): "localhost:" + port2,
	})

	msgChan1, err := network1.Listen()
	if err != nil {
		t.Fatalf("network1 listen failed: %v", err)
	}

	msgChan2, err := network2.Listen()
	if err != nil {
		t.Fatalf("network2 listen failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	msg := protocol.Message{
		Sender: crdt.NodeId("node1"),
		Payload: protocol.Command{
			Type: protocol.Propose,
		},
	}

	err = network1.Send(crdt.NodeId("node2"), msg)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	select {
	case received := <-msgChan2:
		if received.Sender != msg.Sender {
			t.Errorf("expected sender %s, got %s", msg.Sender, received.Sender)
		}
		if received.Payload.Type != msg.Payload.Type {
			t.Errorf("expected command type %v, got %v", msg.Payload.Type, received.Payload.Type)
		}

		reply := protocol.Message{
			Sender: crdt.NodeId("node2"),
			Payload: protocol.Command{
				Type: protocol.Ack,
			},
		}
		err = network2.Send(crdt.NodeId("node1"), reply)
		if err != nil {
			t.Fatalf("reply send failed: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no message received on network2")
	}

	select {
	case received := <-msgChan1:
		if received.Sender != crdt.NodeId("node2") {
			t.Errorf("expected sender node2, got %s", received.Sender)
		}
		if received.Payload.Type != protocol.Ack {
			t.Errorf("expected Ack, got %v", received.Payload.Type)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no reply received on network1")
	}

	network1.Close()
	network2.Close()
}
