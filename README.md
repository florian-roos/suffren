# Suffren: Distributed Rate Limiter Powered by Lattice Agreement

## The Problem

Imagine you run an API gateway with multiple instances behind a load balancer. You need to enforce rate limits like "each user can make at most 100 requests per minute." On a single server, this is trivial: a counter and a timestamp. But across multiple instances, the counter must be shared. Where do you store it?

**Option 1: A central database (e.g., Redis).** Every request triggers a network round-trip to check and increment the counter. The database becomes a single point of failure and a throughput bottleneck. If it goes down, either rate limiting stops working or your entire API does.

**Option 2: Gossip-based eventual consistency.** Each node maintains its own copy of the counters and periodically syncs with peers. This is fast and fault-tolerant, but there is no guarantee on _when_ the counters converge. Two nodes might simultaneously believe a user is under the limit, both allow the request, and the real count overshoots. There is no commit barrier.

**Option 3: Full consensus (e.g., Raft/Paxos).** A leader totally orders all operations. This is safe, but it is overkill for monotonic counters: you don't need a total order, you just need to know the maximum. Leader election adds latency and complexity, and the leader is a bottleneck under load.

Suffren explores a fourth approach.

## What Is Suffren?

Suffren is a distributed rate limiter that uses **Lattice Agreement**, a consensus protocol designed specifically for monotonic state (counters that only grow). Instead of enforcing a total order like Paxos, it only guarantees that all nodes eventually agree on the _join_ (component-wise maximum) of all proposed values. This is a weaker (and therefore faster) guarantee than total-order consensus, but it is sufficient for counters.

The result: any node can accept a rate limit decision locally, propagate it to the cluster, and receive a deterministic confirmation that a quorum has adopted the value. No leader, no single point of failure, and a strict synchronization barrier that gossip cannot provide.

### Key Features

- **No central coordinator**: All nodes are equal; any node can accept and propagate rate limit decisions.
- **Linearizable reads and writes**: A `Check` or `Status` call blocks until a quorum has confirmed the value, so clients never see stale data.
- **Fault-tolerant**: Tolerates network partitions and node crashes (as long as a quorum remains reachable).
- **Sliding-window rate limiting**: Supports per-identifier, per-resource limits with configurable time windows.
- **Persistence & Crash Recovery**: Nodes automatically snapshot their state to disk (atomic writes + `fsync`) and recover across restarts.
- **Observability**: Built-in structured JSON logging (`slog`) with automatic latency tracking and request tracing.
- **HTTP API**: Drop-in rate limiting service with a simple JSON API.
- **Standard library networking**: TCP transport with `encoding/gob` serialization; no external messaging dependencies.

## Getting Started

### Prerequisites

- Go 1.24+
- A `.env` file defining the cluster topology (see below)

### Configuration

Create a `.env` file in the working directory (see .env.example)

Each entry is `NodeId:address`. Every node in the cluster must have the same `PEERS` list.

### Running a Local Cluster

Open three terminals and start one node in each:

```bash
go run cmd/suffren/main.go --id N1
go run cmd/suffren/main.go --id N2
go run cmd/suffren/main.go --id N3
```

### Running the HTTP API

To start a node in production (API) mode:

```bash
go run ./cmd/suffren --api --id N1 --api-port 8081
```

| Flag         | Default | Description                                  |
| ------------ | ------- | -------------------------------------------- |
| `--api`      | `false` | Start the HTTP API server instead of the CLI |
| `--id`       | `N1`    | Unique node identifier in the cluster        |
| `--api-port` | `8080`  | Listening port for the HTTP API              |

### Running with Docker

Suffren is containerized with Docker. The image contains a the static binary and an alpine runtime.

To build the image locally:

```bash
docker build -t suffren:latest .
```

To run a node as a Docker container:

```bash
docker run --rm -p 8080:8080 -e PEERS="N1:localhost:8031,N2:localhost:8032,N3:localhost:8033" suffren:latest
```

## HTTP API

### `POST /check`

Checks whether a request is allowed and atomically increments the counter.

**Request:**

```json
{
  "identifier": "user123",
  "resource": "api_requests",
  "limit": 100,
  "window": "60s",
  "value_requested": 1
}
```

**Response (200 OK):**

```json
{
  "allowed": true,
  "current": 1,
  "limit": 100,
  "remaining": 99,
  "reset_at": "2026-07-09T11:01:00Z"
}
```

Returns `503 Service Unavailable` if the cluster times out.

### `POST /status`

Returns the current consumption without incrementing.

**Request:** Same body as `/check` (`value_requested` is ignored).

**Response (200 OK):**

```json
{
  "current": 42,
  "limit": 100,
  "remaining": 58,
  "reset_at": "2026-07-09T11:01:00Z"
}
```

## Interactive CLI

When started without `--api`, Suffren launches an interactive CLI for manual testing:

```text
s              Start the node (bind TCP port, begin protocol)
i <key> [val]  Increment the counter for <key> by [val] (default 1), blocks until LEARN
v <key>        Quorum read of the counter value for <key> (linearizable)
q              Graceful shutdown
```

## Testing

```bash
# Run all tests with the race detector
go test -race ./...

# Run benchmarks
go test -bench=. ./...
```

The CI pipeline (`.github/workflows/ci.yaml`) runs `go vet`, `go test -race`, and `golangci-lint` on every push and pull request to `main`.

## Project Structure

```text
cmd/
  suffren/              # Entry point (CLI and API server)
    cli.go              # Interactive CLI for manual testing
    main.go             # Flag parsing, .env loading, node/API startup

internal/
  config/               # Configuration types and DefaultConfig
  api/                  # HTTP API server (POST /check, POST /status)
  crdt/                 # Lattice interface, GCounter, CounterMap
  engine/               # Counter map engine (Start, IncrementKey, ValueForKey, Stop)
  latticeagreement/     # Proposer, Acceptor, Learner, MessageRouter
  limiter/              # Sliding-window rate limiter on the CRDT counter
  node/                 # Node lifecycle (Start/Stop, message dispatch)
  p2p/                  # TCP transport (Server, Client, Connection, Network)
  protocol/             # Message and Command wire types (gob-encoded)
  retry/                # Jitter, Retry helpers
  storage/              # Disk persistence and snapshotting (FileStorage)
  testutils/            # Local peers generator
```

## Dependencies

- [godotenv](https://github.com/joho/godotenv): `.env` file loading

All other functionality uses the Go standard library.

---

## Architecture

Each node runs three independent roles as separate goroutines, each with its own mailbox (channel). This isolation prevents deadlocks during concurrent network I/O (a proposer waiting for quorum does not block the acceptor from handling incoming proposals from other nodes).

```
┌─────────────────────────────────────────────────────────┐
│                        Node                             │
│                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐     │
│  │  Proposer   │  │  Acceptor   │  │   Learner    │     │
│  │             │  │             │  │              │     │
│  │ Broadcasts  │  │ Validates   │  │ Commits and  │     │
│  │ PROPOSE,    │  │ proposals,  │  │ re-broadcasts│     │
│  │ collects    │  │ replies     │  │ LEARN values │     │
│  │ ACKs/NACKs  │  │ ACK or NACK │  │              │     │
│  └──────┬──────┘  └──────┬──────┘  └──────┬───────┘     │
│         │                │                │             │
│         └────────────────┴────────────────┘             │
│                          │                              │
│                  ┌───────┴───────┐                      │
│                  │ MessageRouter │                      │
│                  │ (dispatches   │                      │
│                  │  by type)     │                      │
│                  └───────┬───────┘                      │
│                          │                              │
│                  ┌───────┴───────┐                      │
│                  │  P2P Network  │                      │
│                  │  (TCP + gob)  │                      │
│                  └───────────────┘                      │
└─────────────────────────────────────────────────────────┘
```

### The Three Roles

**Proposer**: A node wants to propagate its local counter state (e.g., after a rate limit check incremented a counter). It broadcasts `PROPOSE(value)` to all peers. If a quorum of acceptors reply `ACK`, the value is committed and the proposer broadcasts `LEARN`. If any acceptor replies `NACK` (because it has seen a higher value), the proposer merges that value into its proposal and retries. This loop is bounded (each NACK forces a strictly upward move in the lattice, so the proposer can only retry a finite number of times before its value dominates everything).

**Acceptor**: Each node also acts as an acceptor for other nodes' proposals. It maintains the highest value it has accepted. When it receives `PROPOSE(v)`, it checks whether its accepted value is below `v` in the lattice. If so, it accepts and replies `ACK`. If not, it replies `NACK` with its current accepted value, helping the proposer converge. The quorum intersection property guarantees that once a value is learned, any future proposal must overlap with at least one node that has seen it, forcing the proposer to adopt the higher value.

**Learner**: On receiving `LEARN(v)`, if `v` is strictly greater than the node's current learned value, the learner applies it locally and re-broadcasts `LEARN` to all other peers. This gossip-style dissemination ensures reliable delivery even if the original proposer crashes after sending the first `LEARN`. Re-broadcasting only on strict growth prevents infinite loops.

### Rate Limiter Layer

On top of the consensus layer, Suffren implements a **sliding-window counter** rate limiter. Each rate limit rule (identifier + resource + limit + window) is mapped to deterministic CRDT keys based on the window's start timestamp:

```
Key format: <identifier>:<resource>:<window_seconds>:<window_start_RFC3339>
```

For each check, two keys are used (one for the current time window and one for the previous window). The estimated consumption is a weighted sum:

```
total = (previous_window_count × overlap_weight) + current_window_count
```

where `overlap_weight` decreases linearly from 1 to 0 as the current window progresses. This gives a smooth sliding-window approximation without storing individual request timestamps.

The `Check` operation first performs a **local** (non-quorum) read to short-circuit if the limit is already exceeded, avoiding an unnecessary cluster round-trip. Only if the local estimate is under the limit does it increment the counter through the cluster and return the authoritative decision.

## Technical Foundations

This section provides the formal mathematical model and complexity analysis that underpin Suffren's correctness guarantees.

### Lattice Model

The underlying data structure is a `CounterMap` (a map from string keys to `GCounter` instances). Each `GCounter` is itself a map from `NodeId` to a natural number. The `CounterMap` forms a bounded join-semilattice (C, ⊑, ⊔, ⊥):

- **State space:** C = Key → (NodeId → ℕ)
- **Partial order:** c1 ⊑ c2 ⇔ ∀k, ∀n: c1[k][n] ≤ c2[k][n]
- **Join (⊔):** (c1 ⊔ c2)[k][n] = max(c1[k][n], c2[k][n])
- **Bottom (⊥):** The all-zero map

The join operation is **idempotent** (v ⊔ v = v), **commutative** (v ⊔ w = w ⊔ v), and **associative**. This means the order in which proposals arrive does not matter (the final converged value is always the same).

### Linearizability via `onLearn`

When a caller invokes `IncrementKey` or `ValueForKey`, the node registers a pending operation with its proposed value and blocks. The `onLearn` callback fires whenever a new value is learned. It merges the learned value into the local state and signals the pending operation **only if** the learned value dominates the proposed value (`proposedValue ⊑ learnedValue`). This guarantees that the caller's increment is reflected in the committed state before it receives a response (providing linearizable semantics).

### Complexity Analysis

**Time (rounds to commit):**

- **Uncontended (single proposer):** O(1), one round-trip: PROPOSE → quorum of ACKs → LEARN.
- **Maximum contention (N concurrent proposers):** O(N), where each NACK forces the proposer to join a strictly higher value. The lattice height is bounded by the number of distinct proposed values (at most N), so a proposer can receive at most O(N) NACKs before its value dominates all others.

**Messages (worst case, N concurrent proposers):**

Each round for a single proposer costs O(N) messages (1 broadcast to N peers + up to N ACK/NACK replies). With O(N) rounds per proposer and N proposers, the total is O(N³) messages in the worst case.

This is the known complexity of Lattice Agreement with concurrent proposers. It is more expensive than gossip (O(N log N)), but the tradeoff buys a **strict synchronization barrier**: when a proposer receives its LEARN confirmation, it knows with mathematical certainty that a quorum has adopted its value. Gossip cannot provide this guarantee.

**LEARN dissemination:** Each successful proposer broadcasts LEARN to N peers, and each receiving node re-broadcasts once (only on strict growth). This adds O(N²) messages, dominated by the O(N³) proposal phase.

### Fault Tolerance

- **Network partitions:** If a proposer cannot reach a quorum, it times out and the caller receives an error. The cluster continues operating with the reachable majority.
- **Node crashes:** If the original proposer crashes after sending LEARN but before full dissemination, the learner's re-broadcast ensures the value still propagates to all nodes.
- **Mailbox overflow:** If a role's mailbox is full, incoming messages are dropped. This is safe because the proposer will re-converge the cluster on the next round (no data is lost since the CRDT state is monotonic).

## Why "Suffren"?

Admiral Pierre André de Suffren commanded distributed naval fleets that operated independently yet maintained coordination. This is the exact principle behind Lattice Agreement: nodes act autonomously but converge through mathematical guarantees.
