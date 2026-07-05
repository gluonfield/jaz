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
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
)

type historyLimitState struct {
	GroupMessages map[string]int    `json:"group_messages,omitempty"`
	ContactHashes map[string]string `json:"contact_hashes,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

func (p *Provider) writeHistorySync(ctx context.Context, connection integrations.Connection, sync *waHistorySync.HistorySync, now time.Time) error {
	if p.raw == nil {
		return nil
	}
	state, err := p.loadHistoryLimitState(connection.ID)
	if err != nil {
		return err
	}
	records, contacts, groupCounts := whatsappHistoryRecordsLimited(connection, sync, whatsappHistoryCutoff(now), state.GroupMessages, p.groupHistoryLimit())
	contacts, contactHashes := newHistoryContacts(contacts, state.ContactHashes)
	if err := p.writeRecords(ctx, append(records, contacts...)...); err != nil {
		return err
	}
	state.GroupMessages = groupCounts
	state.ContactHashes = contactHashes
	return p.saveHistoryLimitState(connection.ID, state)
}

func (p *Provider) loadHistoryLimitState(connectionID string) (historyLimitState, error) {
	if p.root == "" {
		return emptyHistoryLimitState(), nil
	}
	data, err := os.ReadFile(p.historyLimitStatePath(connectionID))
	if errors.Is(err, os.ErrNotExist) {
		return emptyHistoryLimitState(), nil
	}
	if err != nil {
		return historyLimitState{}, err
	}
	var state historyLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		return historyLimitState{}, err
	}
	if state.GroupMessages == nil {
		state.GroupMessages = map[string]int{}
	}
	if state.ContactHashes == nil {
		state.ContactHashes = map[string]string{}
	}
	return state, nil
}

func emptyHistoryLimitState() historyLimitState {
	return historyLimitState{
		GroupMessages: map[string]int{},
		ContactHashes: map[string]string{},
	}
}

func (p *Provider) saveHistoryLimitState(connectionID string, state historyLimitState) error {
	if p.root == "" {
		return nil
	}
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.historyLimitStatePath(connectionID), data, 0o600)
}

func (p *Provider) historyLimitStatePath(connectionID string) string {
	return filepath.Join(p.root, integrations.NormalizeAlias(connectionID)+".history-limits.json")
}

func (p *Provider) removeHistoryLimitState(connectionID string) error {
	if p.root == "" {
		return nil
	}
	err := os.Remove(p.historyLimitStatePath(connectionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (p *Provider) groupHistoryLimit() int {
	return p.cfg.GroupHistoryLimit
}

func newHistoryContacts(records []integrations.Record, hashes map[string]string) ([]integrations.Record, map[string]string) {
	hashes = copyStringMap(hashes)
	changed := make([]integrations.Record, 0, len(records))
	for _, record := range records {
		hash := historyContactHash(record)
		if hashes[record.ExternalID] == hash {
			continue
		}
		hashes[record.ExternalID] = hash
		changed = append(changed, record)
	}
	return changed, hashes
}

func historyContactHash(record integrations.Record) string {
	sum := sha256.Sum256(record.Raw)
	return hex.EncodeToString(sum[:])
}

func copyStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
