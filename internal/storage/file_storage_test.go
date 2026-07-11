package storage

import (
	"path/filepath"
	"testing"

	"github.com/florian-roos/suffren/internal/crdt"
)

func TestFileStorage_LoadNonExistentFileReturnsEmpty(t *testing.T) {
	// GIVEN: an empty temporary directory
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_state.json")

	store := NewFileStorage(filePath)
	nodeIds := []crdt.NodeId{"N1", "N2"}

	// WHEN: we try to load the state from the non-existent file
	state, err := store.Load(nodeIds)
	if err != nil {
		t.Fatalf("Load on non-existent file returned error: %v", err)
	}

	// THEN: it should successfully return an empty initialized CounterMap
	if state == nil {
		t.Fatal("Expected an initialized CounterMap, got nil")
	}

	if len(state.Counters) != 0 {
		t.Fatalf("Expected empty CounterMap, got %d keys", len(state.Counters))
	}
}

func TestFileStorage_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_state.json")
	nodeIds := []crdt.NodeId{"N1", "N2"}

	// GIVEN: a FileStorage with a populated CounterMap
	store1 := NewFileStorage(filePath)
	state1 := crdt.NewCounterMap(nodeIds)
	state1.IncrementKey("api_key_1", "N1", 10)
	state1.IncrementKey("api_key_1", "N2", 5)
	state1.IncrementKey("api_key_2", "N1", 3)

	// WHEN: the state is saved and then reloaded from a completely new FileStorage instance
	if err := store1.Save(state1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	store2 := NewFileStorage(filePath)
	state2, err := store2.Load(nodeIds)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// THEN: the loaded state should perfectly match the saved state
	val1 := state2.ValueForKey("api_key_1")
	if val1 != 15 {
		t.Errorf("Expected api_key_1 to be 15, got %d", val1)
	}

	val2 := state2.ValueForKey("api_key_2")
	if val2 != 3 {
		t.Errorf("Expected api_key_2 to be 3, got %d", val2)
	}
}

func TestFileStorage_OverwriteExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_state.json")
	nodeIds := []crdt.NodeId{"N1"}

	// GIVEN: a FileStorage with an initially saved state
	store := NewFileStorage(filePath)

	state1 := crdt.NewCounterMap(nodeIds)
	state1.IncrementKey("key", "N1", 1)
	if err := store.Save(state1); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// WHEN: we save a new state to the same FileStorage
	state2 := crdt.NewCounterMap(nodeIds)
	state2.IncrementKey("key", "N1", 5)
	if err := store.Save(state2); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// THEN: loading from the file should return the newest state
	loadedState, err := store.Load(nodeIds)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if val := loadedState.ValueForKey("key"); val != 5 {
		t.Errorf("Expected overwritten key to be 5, got %d", val)
	}
}
