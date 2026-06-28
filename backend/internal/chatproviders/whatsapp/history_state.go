package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
)

type historyLimitState struct {
	GroupMessages map[string]int `json:"group_messages,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at,omitempty"`
}

func (p *Provider) writeHistorySync(ctx context.Context, connection integrations.Connection, sync *waHistorySync.HistorySync, now time.Time) error {
	if p.raw == nil {
		return nil
	}
	state, err := p.loadHistoryLimitState(connection.ID)
	if err != nil {
		return err
	}
	records, groupCounts := whatsappHistoryRecordsLimited(connection, sync, whatsappHistoryCutoff(now), state.GroupMessages, p.groupHistoryLimit())
	if err := p.writeRecords(ctx, records...); err != nil {
		return err
	}
	state.GroupMessages = groupCounts
	return p.saveHistoryLimitState(connection.ID, state)
}

func (p *Provider) loadHistoryLimitState(connectionID string) (historyLimitState, error) {
	if p.root == "" {
		return historyLimitState{GroupMessages: map[string]int{}}, nil
	}
	data, err := os.ReadFile(p.historyLimitStatePath(connectionID))
	if errors.Is(err, os.ErrNotExist) {
		return historyLimitState{GroupMessages: map[string]int{}}, nil
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
	return state, nil
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
