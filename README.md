# Suffren: Quorum-based distributed CRDT via Lattice Agreement

## Overview

Suffren is a replicated distributed counter on N nodes that converges without a central coordinator. Standard consensus algorithms like Paxos enforce a total order through stable leader election, which is overly restrictive for monotonic state. Conversely, gossip protocols provide eventual consistency but lack a deterministic commit barrier. Thus the system cannot mathematically guarantee when a value has fully converged.

Suffren implements a Grow-Only Counter (`GCounter`) using Lattice Agreement. It trades the O(N log N) messaging overhead of gossip for an O(N^2) worst-case complexity to guarantee a strict synchronization barrier. Any node can increment its local state and propose it. When a quorum agrees, every node deterministically adopts the merged value.

## Architecture and Role Isolation

To guarantee consistency during concurrent updates and network partitions, each node implements three strictly isolated roles. Each role operates with independent mutexes to prevent deadlocks during concurrent network I/O.

### 1. Proposer

The proposer initiates agreement rounds. It maintains `bufferedValue`, which is the join (⊔) of all values seen during the current round.

- The proposer broadcasts `PROPOSE(bufferedValue, seqNumber)`.
- If a quorum of `ACK`s is received, the state is committed, and it broadcasts `LEARN`.
- If a `NACK(payload)` is received, it merges the missing state into its buffer. Since the system must work even with a partition of the network, the proposer waits for a quorum of responses, increments `seqNumber`, and re-proposes.

To ensure correct state for late-joining or partitioned nodes, the proposer periodically re-proposes its state.

### 2. Acceptor

The acceptor guarantees the consistency of the accepted values across concurrent proposals. It maintains `acceptedValue`, the ⊔ of all accepted proposals.
For a received `PROPOSE(v)`:

- If `acceptedValue` ⊑ v, the acceptor updates and returns `ACK`.
- If `acceptedValue` ⋢ v, it returns `NACK(acceptedValue)`.
  Because of the quorum intersection property, if a `LEARN` barrier is reached, any future proposal must overlap with a node that has the accepted state, forcing the proposer to adopt the higher lattice state.

### 3. Learner (Commit)

The learner handles the deterministic commit. On receiving `LEARN(v)`, if v strictly dominates the node's `learnedValue`, it applies the operation locally and re-broadcasts `LEARN`. This ensures reliable delivery and prevents the system from blocking if the original proposer crashes before full dissemination.

## Formal Mathematical Model

The underlying object is a `GCounter` mapped to a bounded join-semilattice (C, ⊑, ⊔, ⊥):

- **State space:** C = NodeId → ℕ
- **Partial order:** c1 ⊑ c2 ⇔ ∀k, c1[k] ≤ c2[k]
- **Join operation:** (c1 ⊔ c2)[k] = max(c1[k], c2[k])
- **Bottom element** ⊥: The all-zero map

## Complexity and Fault Tolerance

### Time Complexity (Number of Propose rounds before a new value is commited)

- **Uncontended:** O(1) rounds (1 round-trip: PROPOSE → quorum of ACKs → LEARN).
- **Maximum contention:** O(N) rounds. Each NACK forces a strictly upward lattice move.

### Message Complexity

- **Worst case:** O(N^2). If all N nodes propose simultaneously, the system does not degrade to O(N^3) because quorum intersection ensures that NACK payloads collapse the divergent states rapidly.

### Implementation and Testing

Implemented in Go 1.21+ using strictly the standard library. To verify the safety properties, the system is tested against simulated network failures. The tests can be run with the command : go test -race ./...

## Getting Started

### Running a local 3-node cluster

Open three terminals and run each command in a separate one:

```bash
go run cmd/suffren/main.go 8001
go run cmd/suffren/main.go 8002
go run cmd/suffren/main.go 8003
```

### Interactive CLI

```text
s : Start the node (bind TCP port, begin protocol)
i : Increment local counter and propose to cluster
v : Read current observed value (wait-free)
q : Graceful shutdown
```

## Project Structure

```text
cmd/
  suffren/           # CLI - interactive 3-node demo

internal/
  crdt/              # Lattice interface + GCounter implementation
  lattice-agreement/ # Proposer, Acceptor, Learner, and MessageRouter (one actor per role mailbox model)
  node/              # Start/Stop, periodic proposal, round-timeout
  p2p/               # TCP transport (Server, Client, Connection)
  protocol/          # Message and Command wire types

pkg/
  config/            # Configuration values and Default config
  suffren/           # Public API (NewSuffren, Start, Increment, Value, Stop)
  utils/             # Jitter, Retry
```

### Why "Suffren" ?

Admiral Pierre André de Suffren commanded distributed naval fleets that operated independently yet maintained coordination. This is the exact principle behind Lattice Agreement: nodes act autonomously but converge through mathematical guarantees.
