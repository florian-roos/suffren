package latticeagreement

import (
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"testing"
)

// Helpers

func ack(value *crdt.GCounter) protocol.Message {
	return protocol.Message{
		Sender: "N-acceptor",
		Payload: protocol.Command{
			Type:  protocol.Ack,
			Value: value,
		},
	}
}

func nack(value *crdt.GCounter) protocol.Message {
	return protocol.Message{
		Sender: "N-acceptor",
		Payload: protocol.Command{
			Type:  protocol.Nack,
			Value: value,
		},
	}
}

func peers3() map[crdt.NodeId]string {
	return map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
		"N3": "localhost:8003",
	}
}

// Tests

func TestProposer_bufferedValue_is_join_of_all_proposals_not_last(t *testing.T) {
	// GIVEN: a proposer that has already proposed {A:5, B:0}
	// WHEN:  it proposes {A:0, B:3}
	// THEN:  the PROPOSE broadcast carries {A:5, B:3} (the join), not just {A:0, B:3}
	// This ensures that re-proposals don't lose previously known increments.

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	p := NewProposer(net, "N1", bottom, peers3())

	p.Propose(newTestGCounter(map[crdt.NodeId]uint64{"A": 5, "B": 0}))
	waitForBroadcast(t, net, protocol.Propose)

	net.broadcasted = nil // reset, focus on second proposal

	p.Propose(newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 3}))

	msg, exists := waitForBroadcast(t, net, protocol.Propose)
	if !exists {
		t.Fatal("expected PROPOSE broadcast, got none")
	}

	v := msg.Payload.Value.(*crdt.GCounter)
	if v.Counts["A"] != 5 || v.Counts["B"] != 3 {
		t.Fatalf("expected buffered proposal {A:5,B:3}, got %v", v.Counts)
	}
}

func TestProposer_broadcasts_LEARN_with_bufferedValue_not_original_proposal(t *testing.T) {
	// GIVEN: a proposer that proposes {A:1, B:0}, then receives NACK({A:1, B:7})
	// WHEN:  it re-proposes and reaches a clean quorum (2 ACKs, quorumSize=2)
	// THEN:  the LEARN carries {A:1, B:7} (the join including the NACK payload),
	//        not {A:1, B:0} (the original proposal)
	// This is the critical guarantee: no increment is ever lost.

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	// 2 peers → quorumSize = 2
	p := NewProposer(net, "N1", bottom, map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
	})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0})
	p.Propose(proposed)
	waitForBroadcast(t, net, protocol.Propose)
	net.mu.Lock()
	net.broadcasted = nil
	net.mu.Unlock()

	// NACK with a value that has B:7 (≥ proposed, so proposedValue.IsIn(nackValue))
	nackValue := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 7})
	p.HandleNack(nack(nackValue))
	// ACK with the proposed value to reach quorum (1 NACK + 1 ACK = 2 = quorumSize)
	p.HandleAck(ack(proposed))

	// Re-proposal with buffered value (dirty quorum)
	reproposal, exists := waitForBroadcast(t, net, protocol.Propose)
	if !exists {
		t.Fatal("expected re-PROPOSE after NACK quorum, got none")
	}
	reproposalValue := reproposal.Payload.Value.(*crdt.GCounter)
	if reproposalValue.Counts["B"] != 7 {
		t.Fatalf("re-proposal should carry B:7 from NACK, got %v", reproposalValue.Counts)
	}

	// Now send 2 ACKs for the re-proposed value to reach clean quorum
	p.HandleAck(ack(reproposalValue))
	p.HandleAck(ack(reproposalValue))

	msg, exists := waitForBroadcast(t, net, protocol.Learn)
	if !exists {
		t.Fatal("expected LEARN after clean quorum, got none")
	}

	learnValue := msg.Payload.Value.(*crdt.GCounter)
	if learnValue.Counts["A"] != 1 || learnValue.Counts["B"] != 7 {
		t.Fatalf("LEARN must carry the full bufferedValue {A:1,B:7}, got %v", learnValue.Counts)
	}
}

func TestProposer_stale_ACK_for_old_value_is_ignored(t *testing.T) {
	// GIVEN: a proposer that has moved to a new proposal
	// WHEN:  it receives ACKs matching the OLD proposed value
	// THEN:  those stale ACKs do not count toward quorum

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	p := NewProposer(net, "N1", bottom, peers3())

	oldValue := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0})
	p.Propose(oldValue)
	waitForBroadcast(t, net, protocol.Propose)

	// New proposal overwrites the round
	p.Propose(newTestGCounter(map[crdt.NodeId]uint64{"A": 2, "B": 0}))
	waitForBroadcast(t, net, protocol.Propose)

	net.broadcasted = nil

	// Late ACKs from old round carry the old value — must be ignored
	p.HandleAck(ack(oldValue))
	p.HandleAck(ack(oldValue))

	// No LEARN should have been triggered
	_, learnSent := net.lastBroadcastOfType(protocol.Learn)
	if learnSent {
		t.Fatal("stale ACKs matching old proposed value should not trigger LEARN")
	}
}

func TestProposer_quorum_gate_prevents_duplicate_LEARN(t *testing.T) {
	// GIVEN: a 2-node cluster (quorumSize=2), proposer already reached quorum
	// WHEN:  a third ACK arrives after quorum was already declared
	// THEN:  only one LEARN is broadcast, not two

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	p := NewProposer(net, "N1", bottom, map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
	})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0})
	p.Propose(proposed)
	waitForBroadcast(t, net, protocol.Propose)

	p.HandleAck(ack(proposed))
	p.HandleAck(ack(proposed))
	waitForBroadcast(t, net, protocol.Learn)

	beforeCount := net.countBroadcastsOfType(protocol.Learn)

	// Extra ACKs — should be ignored (quorum already reached)
	p.HandleAck(ack(proposed))
	p.HandleAck(ack(proposed))

	afterCount := net.countBroadcastsOfType(protocol.Learn)
	if afterCount != beforeCount {
		t.Fatalf("expected exactly %d LEARN broadcast(s), got %d after extra ACKs", beforeCount, afterCount)
	}
}

func TestProposer_bufferedValue_accumulates_all_NACK_payloads_across_responses(t *testing.T) {
	// GIVEN: a proposer that receives two NACKs, each with disjoint missing components
	// WHEN:  it reaches quorum (2 NACKs = quorumSize=2) and re-proposes
	// THEN:  the re-proposal carries the join of BOTH nack values, not just the last

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0, "C": 0})
	p := NewProposer(net, "N1", bottom, map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
	})

	proposed := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0, "C": 0})
	p.Propose(proposed)
	waitForBroadcast(t, net, protocol.Propose)
	net.mu.Lock()
	net.broadcasted = nil
	net.mu.Unlock()

	// Two NACKs with disjoint missing components (both ≥ proposed)
	p.HandleNack(nack(newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 5, "C": 0})))
	p.HandleNack(nack(newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0, "C": 9})))

	reproposal, exists := waitForBroadcast(t, net, protocol.Propose)
	if !exists {
		t.Fatal("expected re-PROPOSE after NACK quorum")
	}

	v := reproposal.Payload.Value.(*crdt.GCounter)
	if v.Counts["B"] != 5 || v.Counts["C"] != 9 {
		t.Fatalf("re-proposal must carry join of all NACKs: {A:1,B:5,C:9}, got %v", v.Counts)
	}
}

func TestProposer_new_Propose_resets_round_and_invalidates_previous_quorum(t *testing.T) {
	// GIVEN: a proposer mid-round (1 ACK received, quorumSize=2)
	// WHEN:  Propose() is called again before the round completes
	// THEN:  the old ACK is discarded, the new round starts fresh

	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"A": 0, "B": 0})
	p := NewProposer(net, "N1", bottom, peers3())

	// Start round 1
	oldValue := newTestGCounter(map[crdt.NodeId]uint64{"A": 1, "B": 0})
	p.Propose(oldValue)
	waitForBroadcast(t, net, protocol.Propose)

	p.HandleAck(ack(oldValue))

	// New Propose resets the round
	p.Propose(newTestGCounter(map[crdt.NodeId]uint64{"A": 2, "B": 0}))
	net.broadcasted = nil

	// Late ACK from old round (carries old value) — must be ignored
	p.HandleAck(ack(oldValue))

	_, learnSent := net.lastBroadcastOfType(protocol.Learn)
	if learnSent {
		t.Fatal("stale ACK from old round after Propose() reset should not trigger LEARN")
	}
}
