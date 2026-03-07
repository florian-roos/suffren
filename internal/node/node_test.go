package node

import (
	"suffren/internal/crdt"
	latticeagreement "suffren/internal/lattice-agreement"
	"suffren/internal/protocol"
	"suffren/pkg/config"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Mock Network

type mockNetwork struct {
	mu       sync.Mutex
	messages []protocol.Message
	listenCh chan protocol.Message
	peers    map[crdt.NodeId]string
}

func newMockNetwork(peers map[crdt.NodeId]string) *mockNetwork {
	return &mockNetwork{
		listenCh: make(chan protocol.Message, 100),
		peers:    peers,
	}
}

func (m *mockNetwork) Listen() (<-chan protocol.Message, error) {
	return m.listenCh, nil
}

func (m *mockNetwork) Send(nodeId crdt.NodeId, msg protocol.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockNetwork) Broadcast(msg protocol.Message) error {
	for nodeId := range m.peers {
		err := m.Send(nodeId, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *mockNetwork) BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error {
	for nodeId := range m.peers {
		if nodeId != senderId {
			err := m.Send(nodeId, msg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *mockNetwork) Close() error { return nil }

// Helpers

func newTestNode(t *testing.T, cfg *config.Config) (*Node, *mockNetwork) {
	t.Helper()
	peers := map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
		"N3": "localhost:8003",
	}
	bottom := &crdt.GCounter{Counts: map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0}}
	net := newMockNetwork(peers)
	la := latticeagreement.NewLatticeAgreement("N1", peers, net, bottom, func(crdt.Lattice) {}, &cfg.LatticeAgreement)

	n := NewNode("N1", "8001", peers, net, la, cfg)
	return n, net
}

// Tests

func TestNode_stop_waits_for_goroutines(t *testing.T) {
	// GIVEN: a started node
	// WHEN:  Stop() is called
	// THEN:  it returns only after all goroutines have exited

	n, _ := newTestNode(t, config.DefaultConfig())
	if err := n.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		n.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop() returned cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s (goroutine leak)")
	}
}

func TestNode_no_message_handled_after_stop(t *testing.T) {
	// GIVEN: a stopped node
	// WHEN:  a PROPOSE message arrives after Stop()
	// THEN:  no ACK is produced

	n, net := newTestNode(t, config.DefaultConfig())
	if err := n.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	n.Stop()

	// Record how many messages were sent before the test stimulus
	net.mu.Lock()
	baseline := len(net.messages)
	net.mu.Unlock()

	// Send a PROPOSE after Stop() — the Acceptor actor should be dead
	select {
	case net.listenCh <- protocol.Message{
		Sender: "N2",
		Payload: protocol.Command{
			Type:  protocol.Propose,
			Value: &crdt.GCounter{Counts: map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0}},
		},
	}:
	default:
	}

	time.Sleep(100 * time.Millisecond)

	net.mu.Lock()
	got := len(net.messages)
	net.mu.Unlock()

	if got > baseline {
		t.Fatalf("expected no ACK after Stop(), got %d new message(s)", got-baseline)
	}
}

func TestNode_stop_is_idempotent(t *testing.T) {
	// GIVEN: a started node
	// WHEN:  Stop() is called twice
	// THEN:  no panic

	n, _ := newTestNode(t, config.DefaultConfig())
	if err := n.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop() panicked on second call: %v", r)
		}
	}()

	n.Stop()
	n.Stop() // must not panic
}

func TestNode_concurrent_stop_and_message(t *testing.T) {
	// GIVEN: a started node
	// WHEN:  Stop() and message delivery happen concurrently
	// THEN:  no data race, no panic

	n, net := newTestNode(t, config.DefaultConfig())
	if err := n.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	var wg sync.WaitGroup

	// Flood with messages
	msg := protocol.Message{
		Sender: "N2",
		Payload: protocol.Command{
			Type:  protocol.Propose,
			Value: &crdt.GCounter{Counts: map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0}},
		},
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case net.listenCh <- msg:
			default:
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Stop concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(25 * time.Millisecond)
		n.Stop()
	}()

	wg.Wait()
}

func TestNode_wg_counter_never_negative(t *testing.T) {
	// GIVEN: a node handling many messages
	// WHEN:  Stop() is called while messages are in flight
	// THEN:  wg.Wait() returns cleanly

	cfg := config.DefaultConfig()

	n, net := newTestNode(t, cfg)
	if err := n.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	var sent atomic.Int64
	msg := protocol.Message{
		Sender: "N2",
		Payload: protocol.Command{
			Type:  protocol.Propose,
			Value: &crdt.GCounter{Counts: map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0}},
		},
	}
	go func() {
		for i := 0; i < 200; i++ {
			select {
			case net.listenCh <- msg:
				sent.Add(1)
			default:
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		n.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Stop() hung — likely a wg.Add/Done mismatch, sent %d messages", sent.Load())
	}
}
