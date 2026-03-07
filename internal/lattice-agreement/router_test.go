package latticeagreement

import (
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"suffren/pkg/config"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newStartedLA(t *testing.T, nodeId crdt.NodeId, onLearn func(crdt.Lattice)) (*LatticeAgreement, *mockNetwork) {
	t.Helper()
	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0})
	if onLearn == nil {
		onLearn = func(crdt.Lattice) {}
	}
	la := NewLatticeAgreement(nodeId, peers3(), net, bottom, onLearn, &config.DefaultConfig().LatticeAgreement)
	la.Start()
	t.Cleanup(la.Stop)
	return la, net
}

func newUnstartedLA(t *testing.T) *LatticeAgreement {
	t.Helper()
	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0})
	return NewLatticeAgreement("N1", peers3(), net, bottom, func(crdt.Lattice) {}, &config.DefaultConfig().LatticeAgreement)
}

func TestRouter_each_type_is_enqueued_in_the_correct_mailbox(t *testing.T) {
	// GIVEN: an unstarted LA (mailboxes not drained)
	// WHEN:  messages of various types are sent to HandleIncomingMessage
	// THEN:  each message is enqueued in the correct mailbox, and unknown types are dropped
	cases := []struct {
		name  string
		msg   protocol.Message
		wantP int // proposerMailbox
		wantA int // acceptorMailbox
		wantL int // learnerMailbox
	}{
		{
			name:  "PROPOSE → acceptor",
			msg:   propose("N1", newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0})),
			wantA: 1,
		},
		{
			name:  "ACK → proposer",
			msg:   ack(newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0})),
			wantP: 1,
		},
		{
			name:  "NACK → proposer",
			msg:   nack(newTestGCounter(map[crdt.NodeId]uint64{"N1": 1})),
			wantP: 1,
		},
		{
			name: "LEARN → learner",
			msg: protocol.Message{Sender: "N2", Payload: protocol.Command{
				Type:  protocol.Learn,
				Value: newTestGCounter(map[crdt.NodeId]uint64{"N1": 5}),
			}},
			wantL: 1,
		},
		{
			name: "unknown → dropped",
			msg:  protocol.Message{Sender: "N?", Payload: protocol.Command{Type: protocol.CommandType(99)}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			la := newUnstartedLA(t)
			la.MessageRouter.HandleIncomingMessage(tc.msg)
			if got := len(la.MessageRouter.proposerMailbox); got != tc.wantP {
				t.Errorf("proposerMailbox: want %d, got %d", tc.wantP, got)
			}
			if got := len(la.MessageRouter.acceptorMailbox); got != tc.wantA {
				t.Errorf("acceptorMailbox: want %d, got %d", tc.wantA, got)
			}
			if got := len(la.MessageRouter.learnerMailbox); got != tc.wantL {
				t.Errorf("learnerMailbox: want %d, got %d", tc.wantL, got)
			}
		})
	}
}

func TestRouter_full_mailbox_drops_without_blocking(t *testing.T) {
	// GIVEN: a router with a single-slot acceptor mailbox and no draining goroutines
	// WHEN:  two PROPOSE messages are sent (first fills the slot, second overflows)
	// THEN:  the second call returns immediately (it must not block)

	smallCfg := &config.LatticeAgreementConfig{MsgChanSize: 1}
	net := &mockNetwork{}
	bottom := newTestGCounter(map[crdt.NodeId]uint64{"N1": 0, "N2": 0, "N3": 0})
	// Intentionally NOT started. Mailbox is never drained.
	la := NewLatticeAgreement("N2", peers3(), net, bottom, func(crdt.Lattice) {}, smallCfg)

	fill := propose("N1", newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0}))
	la.MessageRouter.HandleIncomingMessage(fill) // occupies the single slot

	done := make(chan struct{})
	go func() {
		la.MessageRouter.HandleIncomingMessage(fill) // must drop, not block
		close(done)
	}()

	select {
	case <-done:
		// Good — returned immediately.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("HandleIncomingMessage blocked when acceptor mailbox was full")
	}
}

func TestRouter_concurrent_sends_no_race(t *testing.T) {
	// GIVEN: a started LA
	// WHEN:  20 goroutines each send 200 mixed messages via HandleIncomingMessage
	// THEN:  no data race detected by the race detector

	la, _ := newStartedLA(t, "N2", nil)

	msgs := []protocol.Message{
		propose("N1", newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0})),
		{Sender: "N1", Payload: protocol.Command{Type: protocol.Ack, Value: newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0})}},
		{Sender: "N1", Payload: protocol.Command{
			Type:  protocol.Learn,
			Value: newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0}),
		}},
	}

	const senders = 20
	const msgsPerSender = 200
	var wg sync.WaitGroup
	wg.Add(senders)
	for i := 0; i < senders; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < msgsPerSender; j++ {
				la.MessageRouter.HandleIncomingMessage(msgs[j%len(msgs)])
			}
		}(i)
	}
	wg.Wait()
}

func TestRouter_stop_during_message_flood_does_not_panic(t *testing.T) {
	// GIVEN: a started LA with 5 goroutines flooding it with PROPOSE messages
	// WHEN:  Stop() is called after a brief flood
	// THEN:  Stop() returns cleanly and the flood goroutines drain without panic

	la, _ := newStartedLA(t, "N2", nil)

	msg := propose("N1", newTestGCounter(map[crdt.NodeId]uint64{"N1": 1, "N2": 0, "N3": 0}))

	var running atomic.Bool
	running.Store(true)

	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for running.Load() {
				la.MessageRouter.HandleIncomingMessage(msg)
			}
		}()
	}

	time.Sleep(10 * time.Millisecond) // let the flood run briefly
	la.Stop()                         // t.Cleanup will call Stop() again — must be idempotent
	running.Store(false)
	wg.Wait()
}
