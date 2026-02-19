# Suffren - Distributed CRDT Counter

A distributed counter system built with Conflict-free Replicated Data Types (CRDTs) and the Lattice Agreement protocol. Demonstrates quorum-based consensus on lattice-ordered values with bounded convergence guarantees.

## Overview

Suffren implements the Lattice Agreement algorithm for distributed counter consensus. Unlike eventual consistency through gossip, LA provides quorum-based agreement with strong termination guarantees (≤ N+1 rounds). The system uses a GCounter CRDT as its underlying lattice structure.

### Key Features

- **Quorum-based consensus**: Agreement through overlapping quorums (safety guarantee)
- **Bounded termination**: Convergence in at most N+1 proposal rounds
- **Lattice Agreement protocol**: PROPOSE/ACK/NACK/LEARN message flow
- **GCounter as lattice**: Partial order with join operation (⊔)
- **Helping mechanism**: NACK responses include accepted values for fast convergence
- **Static membership**: Fixed N nodes with known addresses (initial version)

## Architecture

### Lattice Agreement Protocol

Suffren implements the Lattice Agreement algorithm, which provides stronger guarantees than passive replication:

**Key Properties:**

- **Quorum intersection**: Safety through overlapping acceptor sets
- **Bounded rounds**: At most N+1 proposal attempts until convergence
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

See [TODO.md](TODO.md) for current progress.

**Phase 1: CRDT Foundation**  
Implement GCounter with lattice operations (join, partial order). Write unit tests.

**Phase 2: Protocol Messages**  
Define PROPOSE/ACK/NACK/LEARN messages. Wire them into the existing p2p layer.

**Phase 3: Lattice Agreement**  
Implement the LA protocol state machine and message handlers in the node.

**Phase 4: End-to-End Testing**  
Run 3-node cluster locally. Test sequential and concurrent proposals. Verify convergence.

## Design Decisions

### Why Lattice Agreement over Simple Gossip?

**LA provides stronger guarantees:**

- Bounded convergence time (≤ N+1 rounds vs unbounded)
- Quorum-based safety (vs eventual consistency)
- Active consensus (vs passive replication)

**Trade-off:** Higher message complexity (O(N²) per proposal) vs weaker consistency

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

**Future:** Can extend to dynamic membership with view changes

### Why Map-based Counter?

Dynamic key space: nodes can be added to lattice without recompilation. Sparse representation saves memory and network bandwidth.

## Building Blocks

This project leverages:

- **Go** for performance and concurrency primitives
- **Lattice theory** for mathematical correctness guarantees

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
