package store

import "sync"

type Store struct {
	mu sync.RWMutex
	kv map[string]string //Key value
}

func NewStore() *Store {
	return &Store{
		mu: sync.RWMutex{},
		kv: make(map[string]string),
	}
}

func (s *Store) Put(key string, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv[key] = value
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.kv[key]
	return value, exists
}
