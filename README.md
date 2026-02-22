# Suffren - Distributed CRDT Counter

A distributed counter system built with Conflict-free Replicated Data Types (CRDTs) and the Lattice Agreement protocol. Demonstrates quorum-based consensus on lattice-ordered values with bounded convergence guarantees.

## Overview

Suffren implements the Lattice Agreement algorithm for distributed counter consensus. Unlike eventual consistency through gossip, LA provides quorum-based agreement with strong termination guarantees (Each proposal instance terminates in at most N+1 attempts. Every received value is eventually learnt in O(N) message delays). The system uses a GCounter CRDT as its underlying lattice structure.

### Key Features

- **Quorum-based consensus**: Agreement through overlapping quorums (safety guarantee)
- **Bounded termination**: Convergence of a proposal instance in at most N+1 proposal rounds
- **Lattice Agreement protocol**: PROPOSE/ACK/NACK/LEARN message flow
- **GCounter as lattice**: Partial order with join operation (⊔)
- **Helping mechanism**: NACK responses include accepted values for fast convergence
- **Static membership**: Fixed N nodes with known addresses (initial version)

## Architecture

### Lattice Agreement Protocol

Suffren implements the Lattice Agreement algorithm, which provides stronger guarantees than passive replication:

**Key Properties:**

- **Quorum intersection**: Safety through overlapping acceptor sets
- **Bounded rounds**: Each proposal instance terminates in at most N+1 attempts. Every received value is eventually learnt in O(N) message delays.
- **Helping mechanism**: Acceptors share their state to accelerate agreement
- **No central coordinator**: Fully decentralized with symmetric roles

### Protocol Flow

1. **Propose**: Node proposes value `v` (e.g., increment its counter entry)
2. **Accept/Reject**:
   - Send `PROPOSE(v, t, nodeID)` to all N nodes
   - Acceptor replies ACK if `acceptedValue ⊑ v` (compatible)
   - Acceptor replies NACK with `acceptedValue` if incompatible
3. **Learn**:
   - On quorum of ACKs: send `LEARN(bufferedValue)` to all
   - On any NACK: merge buffered values, re-propose with higher sequence number
4. **Adopt**: On receiving LEARN, adopt the learned value

### Node State Variables

Each node maintains (per LA algorithm):

```go
bufferedValue  = ⊥  // Join (⊔) of all known proposals
proposedValue  = ⊥  // The current proposal being processed
acceptedValue  = ⊥  // Join of all accepted proposals
readyToLearn   = false  // Ready to adopt a learned value
sequence       = 0  // Monotonically increasing proposal counter
```

### GCounter as Lattice

The counter forms a join-semilattice:

```go
GCounter = map[NodeID]uint64

// Partial order: c1 ⊑ c2 iff ∀k: c1[k] ≤ c2[k]
// Join operation: (c1 ⊔ c2)[k] = max(c1[k], c2[k])
// Bottom element: ⊥ = {} (empty map)

Example:
  {node1: 5, node2: 3} ⊔ {node1: 3, node2: 4, node3: 2}
= {node1: 5, node2: 4, node3: 2}
```

Total counter value: `sum(counter.values)`

### Static Configuration

Initial deployment with fixed N nodes:

```json
{
  "nodes": [
    { "id": "node1", "address": "localhost:8001" },
    { "id": "node2", "address": "localhost:8002" },
    { "id": "node3", "address": "localhost:8003" }
  ],
  "quorum_size": 2
}
```

Quorum size: `⌊N/2⌋ + 1` (majority)

## Roadmap

**Phase 1: CRDT Foundation**  
Implement GCounter with lattice operations (join, partial order). Write unit tests.

**Phase 2: Protocol Messages**  
Define PROPOSE/ACK/NACK/LEARN messages and the interface to route them. Wire them into the existing p2p layer.

**Phase 3: Lattice Agreement**  
Implement the LA protocol state machine.

**Phase 4: End-to-End Testing**  
Run N-node cluster locally. Test concurrent proposals to verify convergence. Measure convergence latency and number of proposal rounds under contention.

## Design Decisions

### Why Lattice Agreement over Simple Gossip?

With gossip + GCounter, nodes converge eventually through random
peer exchange. There is no defined moment where you can say that all nodes agree on the same value right now.

LA adds a commit point: the LEARN message is the exact moment every node officially agrees. This is what makes it relevant for exemple for cache invalidation because you need to know when a key is invalidated everywhere, not just that it will be eventually.

**Trade-off:** O(N²) total messages in the worst case (N proposals × N messages each) vs O(N log N) total messages for gossip, but without a guaranteed commit point.

### Why GCounter as the Lattice?

- Natural partial order: component-wise comparison
- Trivial join operation: component-wise max
- Commutative, associative, idempotent (CRDT properties)
- Easy to reason about and verify

### Why Static Configuration Initially?

**Simplifies MVP:**

- No dynamic membership protocol needed
- Quorum calculation is fixed
- Testing is deterministic

**Future:** Can extend to dynamic membership with view changes and optimization on message passing

## Project Context

This project demonstrates distributed systems engineering principles, exploring:

- **Lattice theory**: Partial orders, join-semilattices, monotonicity
- **Consensus algorithms**: Quorum-based agreement without central coordination
- **CRDT theory**: Conflict-free replicated data types as lattices
- **Trade-offs**: Message complexity vs consistency guarantees
- **Correctness**: Safety (quorum intersection) and liveness (helping mechanism)

## Why "Suffren"?

Admiral Pierre André de Suffren commanded distributed naval fleets that operated independently yet maintained coordination - the exact principle behind Lattice Agreement: nodes act autonomously but converge through mathematical guarantees.

## License

MIT
