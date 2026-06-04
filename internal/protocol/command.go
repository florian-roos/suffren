package protocol

import "github.com/florian-roos/suffren/internal/crdt"

type CommandType byte

const (
	Propose CommandType = iota
	Ack
	Nack
	Learn
)

type Command struct {
	Type      CommandType
	Value     crdt.Lattice
	SeqNumber uint64
}
