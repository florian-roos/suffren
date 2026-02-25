package crdt

type Lattice interface {
	Join(other Lattice) Lattice
	IsIn(other Lattice) bool
	Bottom() Lattice
}
