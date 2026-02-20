package crdt

type Lattice interface {
	Join(other Lattice)
	IsIn(other Lattice) bool
	Bottom() Lattice
}
