type Lattice interface {
	(*Lattice) Join(other Lattice) 
	(*Lattice) IsIn(other Lattice) bool
	(*Lattice) Bottom() Lattice 
}