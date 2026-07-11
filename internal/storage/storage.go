package storage

import "github.com/florian-roos/suffren/internal/crdt"

// Interface for persisting the CRDT CounterMap to disk.
type Storage interface {
	// writes the entire CounterMap state to persistent storage.
	Save(state *crdt.CounterMap) error

	// retrieves the CounterMap state from persistent storage.
	// If the storage is empty or does not exist, it must return a new, empty CounterMap and no error.
	Load(nodeIds []crdt.NodeId) (*crdt.CounterMap, error)
}

// Dummy storage implementation that does nothing.
// Useful for tests or running entirely in-memory.
type NoopStorage struct{}

func (n *NoopStorage) Save(state *crdt.CounterMap) error {
	return nil
}

func (n *NoopStorage) Load(nodeIds []crdt.NodeId) (*crdt.CounterMap, error) {
	return crdt.NewCounterMap(nodeIds), nil
}
