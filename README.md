# Suffren: Distributed Rate Limiter powered by Lattice Agreement

## Overview

Suffren is a distributed, quorum-based rate limiter built on top of a replicated CRDT counter. Standard consensus algorithms like Paxos enforce a total order through stable leader election, which is overly restrictive for monotonic state. Conversely, gossip protocols provide eventual consistency but lack a deterministic commit barrier, meaning the system cannot mathematically guarantee when a value has fully converged.

Suffren implements a `CounterMap` (a map of named `GCounter` instances) using Lattice Agreement. It trades the O(N log N) messaging overhead of gossip for an O(N^3) worst-case complexity to guarantee a strict synchronization barrier. Any node can increment a counter key and propose it. When a quorum agrees, every node deterministically adopts the merged value.

On top of this consensus layer, Suffren provides a sliding-window rate limiter exposed through a simple HTTP API. Each rate limit decision (identifier + resource + rule) is mapped to deterministic CRDT keys, so the cluster maintains a globally consistent view of consumption across all nodes.

## How It Works

### Distributed Counter (Lattice Agreement)

The underlying object is a `CounterMap` mapped to a bounded join-semilattice (C, ⊑, ⊔, ⊥):

- **State space:** C = Key → (NodeId → ℕ)
- **Partial order:** c1 ⊑ c2 ⇔ ∀k, ∀n: c1[k][n] ≤ c2[k][n]
- **Join operation:** (c1 ⊔ c2)[k][n] = max(c1[k][n], c2[k][n])
- **Bottom element** ⊥: The all-zero map

Each node maintains a local `CounterMap` and proposes it through Lattice Agreement. The `onLearn` callback merges the learned value and unblocks any pending `IncrementKey` or `ValueForKey` operation whose proposed value is contained in the learned state, providing linearizable semantics for the caller.

### Rate Limiter (Sliding Window)

The rate limiter implements a sliding-window counter algorithm on top of the distributed `CounterMap`:

- Each (identifier, resource, rule) tuple is mapped to two deterministic keys: one for the **current** window and one for the **previous** window.
- The estimated total consumption is computed as:

  `total = (previous_count × overlap_weight) + current_count`

  where `overlap_weight = 1 - (elapsed_time_in_current_window / window_duration)`.

- `Check` first performs a **local** read to short-circuit if the limit is already exceeded (avoiding a cluster round-trip). Otherwise, it increments the current window counter through the cluster and returns the decision.
- `Status` performs a read-only quorum query to get the current consumption without incrementing.

## Architecture and Role Isolation

To guarantee consistency during concurrent updates and network partitions, each node implements three strictly isolated roles. Each role operates with independent mutexes and its own mailbox (channel), preventing deadlocks during concurrent network I/O.

### 1. Proposer

The proposer initiates agreement rounds. It maintains `bufferedValue`, which is the join (⊔) of all values seen during the current round.

- The proposer broadcasts `PROPOSE(bufferedValue)`.
- If a quorum of `ACK`s is received, the state is committed, and it broadcasts `LEARN`.
- If a `NACK(payload)` is received, it merges the missing state into its buffer. Since the system must work even during network partitions, the proposer waits for a quorum of responses and re-proposes with the accumulated value.

### 2. Acceptor

The acceptor guarantees the consistency of accepted values across concurrent proposals. It maintains `acceptedValue`, the ⊔ of all accepted proposals.

For a received `PROPOSE(v)`:

- If `acceptedValue` ⊑ v, the acceptor updates and returns `ACK`.
- If `acceptedValue` ⋢ v, it returns `NACK(acceptedValue)` with the join of both values to help the proposer converge.
  Because of the quorum intersection property, if a `LEARN` barrier is reached, any future proposal must overlap with a node that has the accepted state, forcing the proposer to adopt the higher lattice state.

### 3. Learner (Commit)

The learner handles the deterministic commit. On receiving `LEARN(v)`, if v strictly dominates the node's `learnedValue`, it applies the value locally and re-broadcasts `LEARN` to all other peers. This ensures reliable delivery and prevents the system from blocking if the original proposer crashes before full dissemination.

### Message Router

All three roles run as independent goroutines with dedicated mailboxes. The `MessageRouter` dispatches incoming messages to the appropriate role based on the command type (`PROPOSE` → acceptor, `ACK`/`NACK` → proposer, `LEARN` → learner). If a mailbox is full, messages are dropped safely since the proposer will re-converge the cluster.

## Complexity and Fault Tolerance

### Time Complexity (Propose rounds before a new value is committed)

- **Uncontended:** O(1) rounds (1 round-trip: PROPOSE → quorum of ACKs → LEARN).
- **Maximum contention:** O(N) rounds. Each NACK forces a strictly upward lattice move.

### Message Complexity

- **Worst case:** O(N^3) if all N nodes propose simultaneously. (Further optimizations are planned to reduce this complexity.)

## Getting Started

### Prerequisites

- Go 1.24+
- A `.env` file defining the cluster peers (see Configuration below)

### Configuration

Suffren reads cluster configuration from a `.env` file in the working directory. The `PEERS` environment variable defines the cluster topology as a comma-separated list of `NodeId:address` pairs:

```env
PEERS=N1:localhost:8031,N2:localhost:8032,N3:localhost:8033
```

Each node must be started with a unique `--id` matching one of the peer IDs.

### Running a local 3-node cluster

Open three terminals and run each command in a separate one:

```bash
go run cmd/suffren/main.go --id N1
go run cmd/suffren/main.go --id N2
go run cmd/suffren/main.go --id N3
```

Each node reads the `PEERS` map from `.env`, binds its TCP port, and begins the protocol.

### Running the HTTP API server

To start a node in API mode (production use):

```bash
go run cmd/suffren/main.go --api --id N1 --api-port 8081
```

Flags:

| Flag         | Default | Description                                  |
| ------------ | ------- | -------------------------------------------- |
| `--api`      | `false` | Start the HTTP API server instead of the CLI |
| `--id`       | `N1`    | Unique node identifier in the cluster        |
| `--api-port` | `8080`  | Listening port for the HTTP API              |

### HTTP API

The API exposes two endpoints:

#### `POST /check`

Checks whether a request is allowed under the rate limit and increments the counter atomically.

Request body:

```json
{
  "identifier": "user123",
  "resource": "api_requests",
  "limit": 100,
  "window": "60s",
  "value_requested": 1
}
```

Response (`200 OK`):

```json
{
  "allowed": true,
  "current": 1,
  "limit": 100,
  "remaining": 99,
  "reset_at": "2026-07-09T11:01:00Z"
}
```

If the cluster is unavailable (timeout), returns `503 Service Unavailable`.

#### `POST /status`

Returns the current consumption for an identifier and resource without incrementing.

Request body (same as `/check`, `value_requested` is ignored):

```json
{
  "identifier": "user123",
  "resource": "api_requests",
  "limit": 100,
  "window": "60s",
  "value_requested": 0
}
```

Response (`200 OK`):

```json
{
  "current": 42,
  "limit": 100,
  "remaining": 58,
  "reset_at": "2026-07-09T11:01:00Z"
}
```

### Interactive CLI (testing mode)

When started without `--api`, Suffren launches an interactive CLI for manual testing:

```text
s              : Start the node (bind TCP port, begin protocol)
i <key> [val]  : Increment the counter for <key> by [val] (default 1), blocks until LEARN
v <key>        : Quorum read of the counter value for <key> (linearizable)
q              : Graceful shutdown
```

## Testing

The system is tested against simulated network failures to verify safety properties. The test suite includes unit tests, race detection, and benchmarks.

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
  suffren/              # Entry point — CLI and API server
    cli.go              # Interactive CLI for manual testing
    main.go             # main(): flag parsing, .env loading, node/API startup

internal/
  api/                  # HTTP API server (POST /check, POST /status)
  crdt/                 # Lattice interface, GCounter, CounterMap
  lattice-agreement/    # Proposer, Acceptor, Learner, MessageRouter (actor/mailbox model)
  node/                 # Node lifecycle (Start/Stop, incoming message dispatch)
  p2p/                  # TCP transport (Server, Client, Connection, Network)
  protocol/             # Message and Command wire types (gob-encoded)
  ratelimiter/          # Sliding-window rate limiter built on the CRDT counter

pkg/
  config/               # Configuration types and DefaultConfig
  suffren/              # Public API (NewSuffren, Start, IncrementKey, ValueForKey, Stop)
  utils/                # Jitter, Retry helpers
```

## Dependencies

- [godotenv](https://github.com/joho/godotenv) — `.env` file loading

All other functionality uses the Go standard library. The wire protocol uses `encoding/gob` for message serialization over TCP.

## Why "Suffren"?

Admiral Pierre André de Suffren commanded distributed naval fleets that operated independently yet maintained coordination. This is the exact principle behind Lattice Agreement: nodes act autonomously but converge through mathematical guarantees.
