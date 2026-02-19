# TODO

## Phase 1: GCounter CRDT

- [ ] Create `internal/crdt/gcounter.go`
- [ ] Implement GCounter struct (map[NodeID]uint64)
- [ ] Implement Join operation (⊔)
- [ ] Implement LessOrEqual (⊑) for partial order
- [ ] Write tests for lattice properties (commutativity, associativity, idempotence)

## Phase 2: Protocol Messages

- [ ] Define ProposeMsg, AckMsg, NackMsg, LearnMsg in `internal/protocol/`
- [ ] Add JSON serialization
- [ ] Route messages through existing p2p handlers

## Phase 3: Lattice Agreement

- [ ] Add LA state to Node struct (buffered/proposed/accepted values, sequence)
- [ ] Implement Propose() method
- [ ] Implement handlePropose (check ⊑, reply ACK/NACK)
- [ ] Implement handleAck/handleNack (collect responses, re-propose if needed)
- [ ] Implement handleLearn (adopt learned value)
- [ ] Load static config (node list, quorum size)

## Phase 4: Testing

- [ ] Create 3-node config.json
- [ ] Test: sequential proposals converge correctly
- [ ] Test: concurrent proposals trigger NACK and re-proposal
- [ ] Test: verify all nodes reach same final state

## Future Enhancements

- [ ] Dynamic membership (view changes)
- [ ] PN-Counter (support decrements)
- [ ] Delta-state optimization (send deltas not full state)
- [ ] Optimistic fast path (skip quorum if no conflicts)
- [ ] Persistent storage (survive crashes)
- [ ] HTTP API for external clients
- [ ] Visualization dashboard
- [ ] Formal verification with TLA+
