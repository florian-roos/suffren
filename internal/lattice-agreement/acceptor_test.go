package latticeagreement

import (
	"runtime"
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
	"testing"
	"time"
)

// Mock network

type mockNetwork struct {
	mu          sync.Mutex
	sent        []capturedMessage
	broadcasted []protocol.Message
	sendErr     error
}

type capturedMessage struct {
	to  crdt.NodeId
	msg protocol.Message
}

func (m *mockNetwork) Send(to crdt.NodeId, msg protocol.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, capturedMessage{to: to, msg: msg})
	return m.sendErr
}

func (m *mockNetwork) Broadcast(msg protocol.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcasted = append(m.broadcasted, msg)
	return m.sendErr
}

func (m *mockNetwork) BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcasted = append(m.broadcasted, msg)
	return m.sendErr
}

func (m *mockNetwork) lastSentTo(nodeId crdt.NodeId) (protocol.Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].to == nodeId {
			return m.sent[i].msg, true
		}
	}
	return protocol.Message{}, false
}

func (m *mockNetwork) lastBroadcastOfType(cmdType protocol.CommandType) (protocol.Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.broadcasted) - 1; i >= 0; i-- {
		if m.broadcasted[i].Payload.Type == cmdType {
			return m.broadcasted[i], true
		}
	}
	return protocol.Message{}, false
}

func (m *mockNetwork) countBroadcastsOfType(cmdType protocol.CommandType) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, msg := range m.broadcasted {
		if msg.Payload.Type == cmdType {
			count++
		}
	}
	return count
}

// Helpers

func newTestGCounter(counts map[crdt.NodeId]uint64) *crdt.GCounter {
	g := &crdt.GCounter{Counts: make(map[crdt.NodeId]uint64)}
	for k, v := range counts {
		g.Counts[k] = v
	}
	return g
}

func propose(proposer crdt.NodeId, value *crdt.GCounter) protocol.Message {
	return protocol.Message{
		Sender: proposer,
		Payload: protocol.Command{
			Type:  protocol.Propose,
			Value: value,
		},
	}
}

// waitForMessage polls the mockNetwork until a message to the given node appears
// or the test times out. Needed because HandlePropose sends asynchronously.
func waitForMessage(t *testing.T, net *mockNetwork, to crdt.NodeId) (protocol.Message, bool) {
	t.Helper()
	deadline := 100
	for i := 0; i < deadline; i++ {
		if msg, exists := net.lastSentTo(to); exists {
			return msg, true
		}
		time.Sleep(1 * time.Millisecond)
		runtime.Gosched()
	}
	return protocol.Message{}, false
}

// waitForBroadcast polls until a broadcast of the given type appears.
func waitForBroadcast(t *testing.T, net *mockNetwork, cmdType protocol.CommandType) (protocol.Message, bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if msg, exists := net.lastBroadcastOfType(cmdType); exists {
			return msg, true
		}
		time.Sleep(1 * time.Millisecond)
		runtime.Gosched()
	}
	return protocol.Message{}, false
}

// Tests

func TestAcceptor_ACKs_when_accepted_is_dominated_by_proposal(t *testing.T) {
	// GIVEN: an acceptor with acceptedValue = {A:1, B:0}
	// WHEN:  it receives PROPOSE({A:3, B:2}) which dominates acceptedValue
	// THEN:  it sends ACK, acceptedValue becomes {A:3, B:2}

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	acceptor := NewAcceptor(net, bottom, "N2")
	acceptor.acceptedValue = newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 3, "B": 2})
	acceptor.HandlePropose(propose("N1", proposed))

	// Wait for the goroutine to send
	msg, exists := waitForMessage(t, net, "N1")
	if !exists {
		t.Fatal("expected a reply to N1, got none")
	}
	if msg.Payload.Type != protocol.Ack {
		t.Fatalf("expected ACK, got %v", msg.Payload.Type)
	}
	if msg.Sender != "N2" {
		t.Fatalf("expected sender N2, got %s", msg.Sender)
	}

	// ACK must echo back the proposed value
	ackValue := msg.Payload.Value.(*crdt.GCounter)
	if ackValue.Counts["A"] != 3 || ackValue.Counts["B"] != 2 {
		t.Fatalf("expected ACK value={A:3,B:2}, got %v", ackValue.Counts)
	}

	// acceptedValue must have been updated
	accepted := acceptor.acceptedValue.(*crdt.GCounter)
	if accepted.Counts["A"] != 3 || accepted.Counts["B"] != 2 {
		t.Fatalf("expected acceptedValue={A:3,B:2}, got %v", accepted.Counts)
	}
}

func TestAcceptor_NACKs_when_proposal_is_incomparable(t *testing.T) {
	// GIVEN: an acceptor with acceptedValue = {A:3, B:0}
	// WHEN:  it receives PROPOSE({A:0, B:5}), which is incomparable
	// THEN:  it sends NACK with the JOIN = {A:3, B:5}
	//        and acceptedValue becomes {A:3, B:5}

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	acceptor := NewAcceptor(net, bottom, "N2")
	acceptor.acceptedValue = newTestGCounter(map[crdt.NodeId]uint64{"A": 3, "B": 0})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 5})
	acceptor.HandlePropose(propose("N1", proposed))

	msg, exists := waitForMessage(t, net, "N1")
	if !exists {
		t.Fatal("expected a reply to N1, got none")
	}
	if msg.Payload.Type != protocol.Nack {
		t.Fatalf("expected NACK, got %v", msg.Payload.Type)
	}

	// The NACK payload must be the join: {A:3, B:5}
	nackValue := msg.Payload.Value.(*crdt.GCounter)
	if nackValue.Counts["A"] != 3 || nackValue.Counts["B"] != 5 {
		t.Fatalf("expected NACK value={A:3,B:5}, got %v", nackValue.Counts)
	}

	// acceptedValue must have grown to the join
	accepted := acceptor.acceptedValue.(*crdt.GCounter)
	if accepted.Counts["A"] != 3 || accepted.Counts["B"] != 5 {
		t.Fatalf("expected acceptedValue={A:3,B:5}, got %v", accepted.Counts)
	}
}

func TestAcceptor_NACKs_when_proposal_is_strictly_dominated(t *testing.T) {
	// GIVEN: an acceptor with acceptedValue = {A:5, B:5}
	// WHEN:  it receives PROPOSE({A:1, B:1}), which is strictly dominated
	// THEN:  it sends NACK with acceptedValue unchanged = {A:5, B:5}
	// This ensures the proposer is forced to catch up.

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	acceptor := NewAcceptor(net, bottom, "N2")
	acceptor.acceptedValue = newTestGCounter(map[crdt.NodeId]uint64{"A": 5, "B": 5})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 1})
	acceptor.HandlePropose(propose("N1", proposed))

	msg, exists := waitForMessage(t, net, "N1")
	if !exists {
		t.Fatal("expected a reply to N1, got none")
	}
	if msg.Payload.Type != protocol.Nack {
		t.Fatalf("expected NACK, got %v", msg.Payload.Type)
	}

	nackValue := msg.Payload.Value.(*crdt.GCounter)
	if nackValue.Counts["A"] != 5 || nackValue.Counts["B"] != 5 {
		t.Fatalf("expected NACK value={A:5,B:5} (unchanged), got %v", nackValue.Counts)
	}
}

func TestAcceptor_ACKs_when_proposal_equals_accepted(t *testing.T) {
	// GIVEN: an acceptor with acceptedValue = {A:3, B:3}
	// WHEN:  it receives PROPOSE({A:3, B:3}) — equal, so accepted ⊑ proposed
	// THEN:  it sends ACK (equal counts as ⊑, not strictly less)

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	acceptor := NewAcceptor(net, bottom, "N2")
	acceptor.acceptedValue = newTestGCounter(map[crdt.NodeId]uint64{"A": 3, "B": 3})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 3, "B": 3})
	acceptor.HandlePropose(propose("N1", proposed))

	msg, exists := waitForMessage(t, net, "N1")
	if !exists {
		t.Fatal("expected a reply to N1, got none")
	}
	if msg.Payload.Type != protocol.Ack {
		t.Fatalf("expected ACK for equal values, got %v (IsIn must return true for equal)", msg.Payload.Type)
	}
}

func TestAcceptor_acceptedValue_is_monotonically_non_decreasing(t *testing.T) {
	// GIVEN: an acceptor receiving a sequence of proposals in non-monotone order
	// WHEN:  proposals arrive out of order (simulating network reordering)
	// THEN:  acceptedValue never decreases

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	acceptor := NewAcceptor(net, bottom, "N2")

	proposals := []*crdt.GCounter{
		newTestGCounter(map[crdt.NodeId]uint64{"A": 5, "B": 0}),
		newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0}), // stale, dominated
		newTestGCounter(map[crdt.NodeId]uint64{"A": 3, "B": 3}),
		newTestGCounter(map[crdt.NodeId]uint64{"A": 2, "B": 7}), // incomparable with previous
	}

	prevValue := bottom
	for i, p := range proposals {
		acceptor.HandlePropose(propose("N1", p))

		waitForMessage(t, net, "N1")

		current := acceptor.acceptedValue.(*crdt.GCounter)
		for nodeId, count := range current.Counts {
			if count < prevValue.Counts[nodeId] {
				t.Fatalf(
					"acceptedValue decreased at proposal %d: node %s went from %d to %d",
					i+1, nodeId, prevValue.Counts[nodeId], count,
				)
			}
		}
		// Deep copy for next iteration comparison
		prevValue = newTestGCounter(current.Counts)
	}
}

func TestAcceptor_concurrent_proposals_do_not_corrupt_state(t *testing.T) {
	// GIVEN: an acceptor receiving many concurrent proposals
	// WHEN:  100000 goroutines each send a proposal simultaneously
	// THEN:  acceptedValue is the join of all proposals (no lost updates, no races)
	// Run with: go test -race

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0, "C": 0})
	acceptor := NewAcceptor(net, bottom, "N2")

	const n = 10000
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			val := newTestGCounter(map[crdt.NodeId]uint64{
				"A": uint64(i),
				"B": uint64(n - i),
				"C": 1,
			})
			acceptor.HandlePropose(propose("N1", val))
		}(i)
	}

	wg.Wait()

	// After all proposals, acceptedValue must be ≥ every individual proposal.
	// The minimum guaranteed value is the join of all proposals:
	// A = max(0..9999) = 9999, B = max(1..10000) = 10000, C = 1
	final := acceptor.acceptedValue.(*crdt.GCounter)
	if final.Counts["A"] != 9999 {
		t.Errorf("expected A=9999, got %d", final.Counts["A"])
	}
	if final.Counts["B"] != 10000 {
		t.Errorf("expected B=10000, got %d", final.Counts["B"])
	}
	if final.Counts["C"] != 1 {
		t.Errorf("expected C=1, got %d", final.Counts["C"])
	}
}
