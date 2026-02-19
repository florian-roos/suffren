# Development Roadmap

## Current Sprint: M1 - GCounter Lattice

### In Progress

- [ ] Implement GCounter struct with map[NodeID]uint64

### Next

- [ ] Implement Join operation (⊔)
- [ ] Implement partial order comparison (⊑)
- [ ] Unit tests for lattice properties
- [ ] Test: commutativity (a⊔b = b⊔a)
- [ ] Test: associativity ((a⊔b)⊔c = a⊔(b⊔c))
- [ ] Test: idempotence (a⊔a = a)

---

## M2: LA Message Types

- [ ] Define message structs in `internal/protocol/`
  - [ ] ProposeMsg (value, sequence, nodeID)
  - [ ] AckMsg (value, sequence, nodeID)
  - [ ] NackMsg (acceptedValue, sequence, nodeID)
  - [ ] LearnMsg (value)
- [ ] Implement JSON marshaling/unmarshaling
- [ ] Add message routing to existing p2p handlers
- [ ] Sequence number generator (atomic increment)

---

## M3: Node State Machine

- [ ] Add LA variables to `internal/node/node.go`:
  - [ ] bufferedValue GCounter
  - [ ] proposedValue GCounter
  - [ ] acceptedValue GCounter
  - [ ] readyToLearn bool
  - [ ] sequence uint64
- [ ] Load static config from JSON file
  - [ ] Parse node list
  - [ ] Calculate quorum size (N/2 + 1)
- [ ] Implement `Propose(v GCounter)` entry point
- [ ] Response collection with timeout
- [ ] Quorum detection logic

---

## M4: LA Protocol Implementation

### Handlers

- [ ] `handlePropose(msg ProposeMsg)` in `message-handler.go`
  - [ ] Check if acceptedValue ⊑ msg.Value
  - [ ] If yes: update acceptedValue, send ACK
  - [ ] If no: compute acceptedValue ⊔ msg.Value, send NACK

- [ ] `handleAckNack()` - collect responses
  - [ ] Wait for quorum responses
  - [ ] If all ACK: broadcast LEARN, return value
  - [ ] If any NACK: buffer values, increment sequence, re-propose

- [ ] `handleNack(msg NackMsg)`
  - [ ] bufferedValue := bufferedValue ⊔ msg.AcceptedValue

- [ ] `handleLearn(msg LearnMsg)`
  - [ ] If proposedValue ⊑ msg.Value and readyToLearn:
  - [ ] Forward LEARN to all (reliable broadcast)
  - [ ] Adopt value locally

### Edge Cases

- [ ] Handle duplicate messages (sequence number dedup)
- [ ] Handle out-of-order messages
- [ ] Timeout for unresponsive nodes

---

## M5: Integration Test

- [ ] Create `config.json` with 3 nodes
- [ ] Start 3 instances on different ports
- [ ] Test 1: Sequential proposals
  - [ ] Node1 proposes {node1: 1}
  - [ ] Wait for learn
  - [ ] Node2 proposes {node2: 1}
  - [ ] Verify all see {node1: 1, node2: 1}

- [ ] Test 2: Concurrent proposals
  - [ ] Node1, Node2, Node3 propose simultaneously
  - [ ] Verify convergence to join of all values
  - [ ] Count number of rounds taken

- [ ] Test 3: Conflicting proposals
  - [ ] Create scenario where NACK is triggered
  - [ ] Verify re-proposal works
  - [ ] Verify bounded termination (≤ 4 rounds for N=3)

- [ ] Test 4: Network delays
  - [ ] Add artificial delays to messages
  - [ ] Verify correctness is maintained

---

## M6: Correctness & Performance (Future)

- [ ] Stress test: 1000 rapid proposals
- [ ] Verify quorum intersection prevents conflicts
- [ ] Measure throughput (proposals/sec)
- [ ] Measure latency (time to learn)
- [ ] Add Prometheus metrics
- [ ] Add structured logging
- [ ] Property-based testing with QuickCheck-style framework

---

## Future Enhancements (Brain Dump)

- [ ] Dynamic membership (view changes)
- [ ] PN-Counter (support decrements)
- [ ] Delta-state optimization (send deltas not full state)
- [ ] Optimistic fast path (skip quorum if no conflicts)
- [ ] Persistent storage (survive crashes)
- [ ] HTTP API for external clients
- [ ] Visualization dashboard
- [ ] Formal verification with TLA+

---
