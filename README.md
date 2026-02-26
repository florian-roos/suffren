# Suffren

A coordinator-less, quorum-based distributed CRDT powered by Lattice Agreement.

## Overview

Suffren implements a distributed state machine where $N$ nodes maintain a global `GCounter`. Unlike traditional consensus protocols (Paxos, Raft) that require stable leader election and sequential log replication, Suffren leverages Lattice Agreement to achieve quorum-based convergence purely through the monotonic properties of join-semilattices.

Any node can increment its local state and propose it to the cluster. When a quorum agrees, every node deterministically adopts the merged value.

## The Trade-Off: Lattice Agreement vs. Gossip

While Gossip protocols provide eventual consistency with $\mathcal{O}(N log N)$ messaging overhead, they fail to provide a deterministic commit point. You never mathematically know when a value has fully converged across the cluster.

Suffren trades higher worst-case messaging complexity $\mathcal{O}(N^2)$ for a guaranteed synchronization barrier. The `LEARN` message guarantees that a strict quorum has acknowledged and merged the state. Because of quorum intersection, any future proposal is mathematically guaranteed to subsume this state.

## Architectural Roles & State Isolation

To guarantee protocol safety and prevent state corruption under high concurrent load, each node implements three strictly isolated roles. Suffren physically separates them into distinct structs with independent mutexes.

### 1. Proposer (Optimistic Forward Progress)

Initiates agreement rounds and drives the cluster toward convergence.

- **State (`bufferedValue`):** The $\sqcup$ (join) of all values seen during the current round, including its own proposals and payloads from `NACK` responses.
- **Mechanics:** Broadcasts `PROPOSE(bufferedValue, seqNumber)`. If a quorum of `ACK`s is received, it broadcasts `LEARN`. If any `NACK` is received, it merges the `NACK` payload and whenever a quorum of responses is reached, increments the sequence number and reproposes.
- **Safety:** `seqNumber` is strictly scoped to the current proposal epoch to safely discard stale messages from delayed network partitions.

### 2. Acceptor (The Consistency Gate)

Guards consistency across concurrent proposals.

- **State (`acceptedValue`):** The $\sqcup$ of all proposals this specific node has accepted. Monotonically non-decreasing.
- **Mechanics:** On receiving `PROPOSE(v)`:
  - If `acceptedValue` $\sqsubseteq v$, it accepts the proposal and replies `ACK`.
  - If `acceptedValue` $\not\sqsubseteq v$, it replies `NACK(acceptedValue)` to immediately feed missing state back to the Proposer, accelerating convergence.

### 3. Learner (Commit & Relay)

Adopts mathematically agreed-upon values.

- **State (`learnedValue`):** The last safely committed value.
- **Mechanics:** On receiving `LEARN(v)`, if $v$ strictly dominates `learnedValue`, the node adopts it, triggers an asynchronous callback to update the client-facing `localCounter` and rebroadcasts the `LEARN` message for reliable delivery.

## Formal Mathematical Model

The underlying data structure is a `GCounter`, mapped to a bounded join-semilattice.

- **State Space:** A map of node IDs to monotonic counters.
- **Partial Order:** $c_1 \sqsubseteq c_2 \iff \forall k, c_1[k] \leq c_2[k]$
- **Join Operation:** $(c_1 \sqcup c_2)[k] = \max(c_1[k], c_2[k])$
- **Bottom Element ($\bot$):** The zero-value map.

## Complexity Analysis

Suffren's performance profile is deterministic. Complexities are defined where N is the total number of nodes in the cluster.

### Time Complexity (Latency)

- **Best Case (Uncontended):** $\mathcal{O}(1)$ rounds. A proposal requires exactly 1 network round-trip (Broadcast PROPOSE -> wait for Quorum of ACKs) before the LEARN commit point is reached.
- **Worst Case (Maximum Contention):** $\mathcal{O}(N)$ rounds. If multiple nodes propose concurrently, a Proposer may receive mixed ACK/NACK quorums. Because every NACK payload forces a strictly upward movement in the lattice, and there are at most N concurrent proposals, a node will reach consensus in at most N proposal rounds.

### Message Complexity

- **Single Proposal (Best Case):** O(N) messages. One PROPOSE broadcast to N nodes, returning a quorum of ACK replies, followed by one LEARN broadcast.
- **Single Proposal (Worst Case for a single node):** $\mathcal{O}(N^2)$ messages. In a highly adversarial network schedule, a single node might be forced to repropose up to N times, discovering exactly one concurrent proposal per round. Each round generates O(N) network messages.
- **System-Wide Avalanche (Maximum Contention):** $\mathcal{O}(N^2)$ messages. If all N nodes propose simultaneously, the system does not degrade to $\mathcal{O}(N^3)$. Due to the quorum intersection property, any two quorums share at least one Acceptor. NACK payloads act as an accelerated gossip mechanism, transmitting the join of multiple proposals at once. This causes the state space to collapse rapidly, bounding the total number of system-wide communication rounds and capping total messages at $\mathcal{O}(N^2)$.

### Space Complexity

- **Memory Footprint:** O(N). The state maintained by the Proposer, Acceptor, and Learner structs (`bufferedValue`, `acceptedValue`, `learnedValue`) is a `GCounter`. A `GCounter` is a map of Node IDs to monotonic integers, bounded strictly by the number of cluster participants. The memory footprint remains flat regardless of the number of increments or protocol rounds.

## Project Structure

```text
internal/
  crdt/              # Core Lattice interface and GCounter implementation
  lattice-agreement/ # Isolated Proposer, Acceptor, and Learner structs
  protocol/          # Message envelopes
  p2p/               # TCP transport layer
```

## Why "Suffren"?

Admiral Pierre André de Suffren commanded distributed naval fleets that operated independently yet maintained coordination. This is the exact principle behind Lattice Agreement: nodes act autonomously but converge through mathematical guarantees.

## License

MIT
