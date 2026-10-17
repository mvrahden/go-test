package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SnapshotStore persists inventory snapshots as JSON files in a directory.
type SnapshotStore struct {
	dir    string
	closed bool
}

func OpenSnapshotStore(dir string) (*SnapshotStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &SnapshotStore{dir: dir}, nil
}

func (s *SnapshotStore) Save(name string, inv *Inventory) error {
	if s.closed {
		return fmt.Errorf("snapshot store is closed")
	}
	inv.mu.Lock()
	data, err := json.Marshal(inv.stock)
	inv.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, name+".json"), data, 0o644)
}

func (s *SnapshotStore) Load(name string) (map[string]int, error) {
	if s.closed {
		return nil, fmt.Errorf("snapshot store is closed")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var out map[string]int
	err = json.Unmarshal(data, &out)
	return out, err
}

// Close flushes and marks the store closed. Further calls error.
func (s *SnapshotStore) Close() error {
	s.closed = true
	return nil
}
