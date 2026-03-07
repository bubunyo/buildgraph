package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// Storage handles reading and writing baseline snapshots.
// The path is always provided explicitly by the caller (from config or flag).
type Storage struct{}

func New() *Storage {
	return &Storage{}
}

func (s *Storage) LoadBaseline(path string) (*types.Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var baseline types.Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, err
	}

	return &baseline, nil
}

func (s *Storage) SaveBaseline(baseline *types.Baseline, path string) error {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
