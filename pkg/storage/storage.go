package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bubunyo/buildgraph/pkg/types"
)

// CurrentVersion is the baseline format version this build of buildgraph
// produces and understands.  LoadBaseline rejects baselines with any other
// version so that stale snapshots never silently produce wrong diffs.
const CurrentVersion = "1.0"

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

	if baseline.Version != CurrentVersion {
		return nil, fmt.Errorf(
			"baseline version %q is not supported (expected %q): re-run 'buildgraph generate' to create a fresh baseline",
			baseline.Version, CurrentVersion,
		)
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
