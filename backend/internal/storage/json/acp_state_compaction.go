package jsonstore

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/storage"
)

const largeACPStateBytes = 64 << 10

func (s *Store) CompactACPStates(ctx context.Context) (int, int64, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if err != nil {
		return 0, 0, err
	}
	compacted := 0
	var removed int64
	var scanErr error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return compacted, removed, err
		}
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.SessionsDir(), entry.Name(), "acp_state.json")
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			scanErr = errors.Join(scanErr, fmt.Errorf("inspect %s: %w", entry.Name(), err))
			continue
		}
		if info.Size() < largeACPStateBytes {
			continue
		}
		changed, saved, err := s.compactACPState(path)
		if err != nil {
			scanErr = errors.Join(scanErr, fmt.Errorf("compact %s: %w", entry.Name(), err))
			continue
		}
		if changed {
			compacted++
			removed += saved
		}
	}
	return compacted, removed, scanErr
}

func (s *Store) compactACPState(path string) (bool, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if len(before) < largeACPStateBytes {
		return false, 0, nil
	}
	var state storage.ACPState
	if err := stdjson.Unmarshal(before, &state); err != nil {
		return false, 0, err
	}
	after, err := stdjson.MarshalIndent(state.WithoutTranscript(), "", "  ")
	if err != nil || len(after) >= len(before) {
		return false, 0, err
	}
	if err := writeACPState(path, after); err != nil {
		return false, 0, err
	}
	return true, int64(len(before) - len(after)), nil
}

func writeACPState(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".acp-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
