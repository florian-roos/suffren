package latticeagreement

// Integration tests: wire Proposer, Acceptor, and Learner together with a routing network in the memory.
//
// The routingCluster routes messages synchronously based on their type:
//   PROPOSE → target node's Acceptor
//   ACK/NACK → target node's Proposer (OriginalProposer field)
//   LEARN  → all nodes' Learners (broadcast)

import (
	"suffren/internal/crdt"
	"suffren/internal/protocol"
	"sync"
	"testing"
	"time"
)

// clusterNode holds the LA components for a single node in the test cluster.
type clusterNode struct {
	id      crdt.NodeId
	la      *LatticeAgreement
	learned crdt.Lattice
	mu      sync.Mutex
}

// routingCluster wires N clusterNodes together in the memory.
type routingCluster struct {
	nodes map[crdt.NodeId]*clusterNode
}

func newCluster(nodeIds []crdt.NodeId) *routingCluster {
	peers := make(map[crdt.NodeId]string)
	for _, id := range nodeIds {
		peers[id] = "" // address unused in in-memory routing
	}

	c := &routingCluster{nodes: make(map[crdt.NodeId]*clusterNode)}

	for _, id := range nodeIds {
		nodeId := id // capture for closure
		cn := &clusterNode{id: nodeId}
		net := &clusterNetwork{cluster: c, selfId: nodeId}
		cn.la = NewLatticeAgreement(nodeId, peers, net, newTestGCounter(map[crdt.NodeId]uint64{
			"N1": 0, "N2": 0, "N3": 0,
		}), func(v crdt.Lattice) {
			cn.mu.Lock()
			defer cn.mu.Unlock()
			if cn.learned == nil {
				cn.learned = v
			} else {
				cn.learned = cn.learned.Join(v)
			}
		})
		c.nodes[nodeId] = cn
	}
	return c
}

// clusterNetwork implements the Network interface and routes via the cluster.
type clusterNetwork struct {
	cluster *routingCluster
	selfId  crdt.NodeId
}

func (n *clusterNetwork) Broadcast(msg protocol.Message) error {
	for _, cn := range n.cluster.nodes {
		n.cluster.dispatch(cn, msg)
	}
	return nil
}

func (n *clusterNetwork) BroadcastToOthers(msg protocol.Message, senderId crdt.NodeId) error {
	for id, cn := range n.cluster.nodes {
		if id != senderId {
			n.cluster.dispatch(cn, msg)
		}
	}
	return nil
}

func (n *clusterNetwork) Send(nodeId crdt.NodeId, msg protocol.Message) error {
	cn, exists := n.cluster.nodes[nodeId]
	if !exists {
		return nil
	}
	n.cluster.dispatch(cn, msg)
	return nil
}

// dispatch routes a message to the right LA component based on its type.
func (c *routingCluster) dispatch(cn *clusterNode, msg protocol.Message) {
	go func() {
		switch msg.Payload.Type {
		case protocol.Propose:
			go cn.la.Acceptors.HandlePropose(msg)
		case protocol.Ack:
			go cn.la.Proposer.HandleAck(msg)
		case protocol.Nack:
			go cn.la.Proposer.HandleNack(msg)
		case protocol.Learn:
			go cn.la.Learner.HandleLearn(msg)
		}
	}()
}

// waitForConvergence polls until all nodes have a learned value or times out.
func waitForConvergence(c *routingCluster, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allLearned := true
		for _, cn := range c.nodes {
			cn.mu.Lock()
			learned := cn.learned
			cn.mu.Unlock()
			if learned == nil {
				allLearned = false
				break
			}
		}
		if allLearned {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// Integration Tests

func TestLA_single_proposal_converges_to_proposed_value(t *testing.T) {
	// GIVEN: a 3-node cluster (quorumSize=2)
	// WHEN:  N1 proposes {N1:3, N2:0, N3:0}
	// THEN:  all nodes eventually learn a value ≥ {N1:3, N2:0, N3:0}
	// This validates the full happy scenario: PROPOSE → ACK → LEARN → onLearn callback.

	c := newCluster([]crdt.NodeId{"N1", "N2", "N3"})

	c.nodes["N1"].la.Proposer.Propose(newTestGCounter(map[crdt.NodeId]uint64{
		"N1": 3, "N2": 0, "N3": 0,
	}))

	if !waitForConvergence(c, 2*time.Second) {
		t.Fatal("cluster did not converge within timeout")
	}

	for id, cn := range c.nodes {
		cn.mu.Lock()
		learned := cn.learned.(*crdt.GCounter)
		cn.mu.Unlock()
		if learned.Counts["N1"] < 3 {
			t.Errorf("node %s learned %v but expected N1≥3", id, learned.Counts)
		}
	}
}

func TestLA_concurrent_proposals_converge_to_join_of_both_values(t *testing.T) {
	// GIVEN: a 3-node cluster
	// WHEN:  N1 proposes {N1:5, N2:0, N3:0} and N2 proposes {N1:0, N2:7, N3:0} concurrently
	// THEN:  all nodes eventually learn a value ≥ {N1:5, N2:7, N3:0}
	// This validates that no proposal is ever lost.

	c := newCluster([]crdt.NodeId{"N1", "N2", "N3"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.nodes["N1"].la.Proposer.Propose(newTestGCounter(map[crdt.NodeId]uint64{
			"N1": 5, "N2": 0, "N3": 0,
		}))
	}()
	go func() {
		defer wg.Done()
		c.nodes["N2"].la.Proposer.Propose(newTestGCounter(map[crdt.NodeId]uint64{
			"N1": 0, "N2": 7, "N3": 0,
		}))
	}()
	wg.Wait()

	if !waitForConvergence(c, 2*time.Second) {
		t.Fatal("cluster did not converge within timeout")
	}

	for id, cn := range c.nodes {
		cn.mu.Lock()
		learned := cn.learned.(*crdt.GCounter)
		cn.mu.Unlock()
		if learned.Counts["N1"] < 5 || learned.Counts["N2"] < 7 {
			t.Errorf("node %s learned %v but expected N1≥5, N2≥7 — concurrent proposals lost", id, learned.Counts)
		}
	}
}

func TestLA_learned_value_is_monotonically_non_decreasing_across_rounds(t *testing.T) {
	// GIVEN: a 3-node cluster that completes multiple rounds
	// WHEN:  each round proposes a value larger than the previous
	// THEN:  the learned value never decreases between rounds
	// Validates that bufferedValue monotonicity holds end-to-end.

	c := newCluster([]crdt.NodeId{"N1", "N2", "N3"})

	rounds := []map[crdt.NodeId]uint64{
		{"N1": 1, "N2": 0, "N3": 0},
		{"N1": 1, "N2": 3, "N3": 0},
		{"N1": 5, "N2": 3, "N3": 2},
	}

	var prevLearned *crdt.GCounter

	for roundIdx, values := range rounds {
		c.nodes["N1"].la.Proposer.Propose(newTestGCounter(values))

		if !waitForConvergence(c, 2*time.Second) {
			t.Fatalf("cluster did not converge in round %d", roundIdx+1)
		}

		cn := c.nodes["N1"]
		cn.mu.Lock()
		learned := cn.learned.(*crdt.GCounter)
		cn.mu.Unlock()

		if prevLearned != nil {
			for nodeId, count := range learned.Counts {
				if count < prevLearned.Counts[nodeId] {
					t.Fatalf("round %d: learned value regressed for node %s: %d → %d",
						roundIdx+1, nodeId, prevLearned.Counts[nodeId], count)
				}
			}
		}
		prevLearned = learned

		// Reset learned state for next round assertion
		for _, node := range c.nodes {
			node.mu.Lock()
			node.learned = nil
			node.mu.Unlock()
		}
	}
}
