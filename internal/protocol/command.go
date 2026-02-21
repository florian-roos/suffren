package protocol

import "suffren/internal/crdt"

type CommandType byte

const (
	Propose CommandType = iota
	Ack
	Nack
	Learn
)

type Command struct {
	Type      CommandType
	Lattice   crdt.Lattice
	SeqNumber uint64
}
