package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type contactSnapshotState struct {
	Hashes    map[string]string `json:"hashes,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
}

func (p *Provider) writeContactSnapshotRecords(ctx context.Context, connection integrations.Connection, records ...integrations.Record) error {
	if len(records) == 0 {
		return nil
	}
	if p.root == "" {
		return p.writeRecords(ctx, records...)
	}
	state, err := p.loadContactSnapshotState(connection.ID)
	if err != nil {
		return err
	}
	if state.Hashes == nil {
		state.Hashes = map[string]string{}
	}
	changed := make([]integrations.Record, 0, len(records))
	for _, record := range records {
		key := record.ExternalID
		if key == "" {
			key = record.ID
		}
		if key == "" {
			changed = append(changed, record)
			continue
		}
		hash := contactRecordHash(record)
		if state.Hashes[key] == hash {
			continue
		}
		state.Hashes[key] = hash
		changed = append(changed, record)
	}
	if len(changed) > 0 {
		if err := p.writeRecords(ctx, changed...); err != nil {
			return err
		}
	}
	return p.saveContactSnapshotState(connection.ID, state)
}

func contactRecordHash(record integrations.Record) string {
	sum := sha256.Sum256(record.Raw)
	return hex.EncodeToString(sum[:])
}

func (p *Provider) loadContactSnapshotState(connectionID string) (contactSnapshotState, error) {
	data, err := os.ReadFile(p.contactSnapshotStatePath(connectionID))
	if errors.Is(err, os.ErrNotExist) {
		return contactSnapshotState{Hashes: map[string]string{}}, nil
	}
	if err != nil {
		return contactSnapshotState{}, err
	}
	var state contactSnapshotState
	if err := json.Unmarshal(data, &state); err != nil {
		return contactSnapshotState{}, err
	}
	if state.Hashes == nil {
		state.Hashes = map[string]string{}
	}
	return state, nil
}

func (p *Provider) saveContactSnapshotState(connectionID string, state contactSnapshotState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := p.contactSnapshotStatePath(connectionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (p *Provider) contactSnapshotStatePath(connectionID string) string {
	return filepath.Join(p.root, integrations.NormalizeAlias(connectionID)+".contact-snapshots.json")
}
