package storage

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/florian-roos/suffren/internal/crdt"
)

// FileStorage implements Storage by snapshotting the CRDT state to a JSON file.
type FileStorage struct {
	filePath string
	mu       sync.Mutex
}

func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		filePath: filePath,
	}
}

// Save writes the CounterMap to disk. It uses an atomic write pattern
// (write to temp file, then rename) to prevent state corruption if the node crashes mid-write.
func (f *FileStorage) Save(state *crdt.CounterMap) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(f.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	tempPath := f.filePath + ".tmp"

	tmpFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}

	// Force write to physical disk before renaming
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, f.filePath); err != nil {
		return err
	}

	// Sync the directory to guarantee the new file entry is persisted
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}

	return nil
}

// Load retrieves the CounterMap from disk.
func (f *FileStorage) Load(nodeIds []crdt.NodeId) (*crdt.CounterMap, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := os.ReadFile(f.filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No state yet, return an empty map
			return crdt.NewCounterMap(nodeIds), nil
		}
		return nil, err
	}

	var state crdt.CounterMap
	if len(data) == 0 {
		return crdt.NewCounterMap(nodeIds), nil
	}

	err = json.Unmarshal(data, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}
