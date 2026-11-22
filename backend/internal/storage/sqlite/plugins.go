package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	pluginsdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/plugins"
)

func (s *Store) ListPluginSources(ctx context.Context) ([]storage.PluginSourceRecord, error) {
	rows, err := pluginsdb.New(s.db).ListPluginSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.PluginSourceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, pluginSourceFromDB(row))
	}
	return out, nil
}

func (s *Store) GetPluginSource(ctx context.Context, id string) (storage.PluginSourceRecord, bool, error) {
	row, err := pluginsdb.New(s.db).GetPluginSource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.PluginSourceRecord{}, false, nil
	}
	if err != nil {
		return storage.PluginSourceRecord{}, false, err
	}
	return pluginSourceFromDB(row), true, nil
}

func (s *Store) SavePluginSource(ctx context.Context, record storage.PluginSourceRecord) error {
	pathsJSON, err := json.Marshal(record.Paths)
	if err != nil {
		return err
	}
	if record.Paths == nil {
		pathsJSON = []byte("[]")
	}
	catalogJSON := string(record.CatalogJSON)
	if catalogJSON == "" {
		catalogJSON = "[]"
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return pluginsdb.New(s.db).UpsertPluginSource(ctx, pluginsdb.UpsertPluginSourceParams{
		ID:            record.ID,
		Repository:    record.Repository,
		Url:           record.URL,
		Ref:           record.Ref,
		Paths:         string(pathsJSON),
		CommitSha:     record.CommitSHA,
		AutoUpdate:    boolInt(record.AutoUpdate),
		IsMarketplace: boolInt(record.IsMarketplace),
		CatalogJson:   catalogJSON,
		CheckedAtMs:   optionalTimeToMs(record.CheckedAt),
		UpdatedAtMs:   timeToMs(updatedAt),
	})
}

func (s *Store) DeletePluginSource(ctx context.Context, id string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := pluginsdb.New(s.db).DeletePluginSource(ctx, id)
	return rows > 0, err
}

func (s *Store) ListPluginStates(ctx context.Context) ([]storage.PluginStateRecord, error) {
	rows, err := pluginsdb.New(s.db).ListPluginStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.PluginStateRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, pluginStateFromDB(row))
	}
	return out, nil
}

func (s *Store) GetPluginState(ctx context.Context, pluginID string) (storage.PluginStateRecord, bool, error) {
	row, err := pluginsdb.New(s.db).GetPluginState(ctx, pluginID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.PluginStateRecord{}, false, nil
	}
	if err != nil {
		return storage.PluginStateRecord{}, false, err
	}
	return pluginStateFromDB(row), true, nil
}

func (s *Store) SavePluginState(ctx context.Context, record storage.PluginStateRecord) error {
	configJSON := []byte("{}")
	if len(record.Config) > 0 {
		var err error
		configJSON, err = json.Marshal(record.Config)
		if err != nil {
			return err
		}
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return pluginsdb.New(s.db).UpsertPluginState(ctx, pluginsdb.UpsertPluginStateParams{
		PluginID:         record.PluginID,
		Enabled:          boolInt(record.Enabled),
		SourceID:         record.SourceID,
		SourceRepository: record.SourceRepository,
		SourceRef:        record.SourceRef,
		SourcePath:       record.SourcePath,
		CommitSha:        record.CommitSHA,
		RemoteSha:        record.RemoteSHA,
		ConfigJson:       string(configJSON),
		UpdatedAtMs:      timeToMs(updatedAt),
	})
}

func (s *Store) DeletePluginState(ctx context.Context, pluginID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return pluginsdb.New(s.db).DeletePluginState(ctx, pluginID)
}

func (s *Store) GetThreadPluginState(ctx context.Context, threadID, pluginID, view, stateKey string) (storage.ThreadPluginStateRecord, bool, error) {
	row, err := pluginsdb.New(s.db).GetThreadPluginState(ctx, pluginsdb.GetThreadPluginStateParams{
		ThreadID: threadID,
		PluginID: pluginID,
		View:     view,
		StateKey: stateKey,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ThreadPluginStateRecord{}, false, nil
	}
	if err != nil {
		return storage.ThreadPluginStateRecord{}, false, err
	}
	return storage.ThreadPluginStateRecord{
		ThreadID:    row.ThreadID,
		PluginID:    row.PluginID,
		View:        row.View,
		StateKey:    row.StateKey,
		DataJSON:    json.RawMessage(row.DataJson),
		UIStateJSON: json.RawMessage(row.UiStateJson),
		Revision:    row.Revision,
		UpdatedAt:   msToTime(row.UpdatedAtMs),
	}, true, nil
}

func (s *Store) ListThreadPluginStatesByPlugin(ctx context.Context, pluginID string) ([]storage.ThreadPluginStateRecord, error) {
	rows, err := pluginsdb.New(s.db).ListThreadPluginStatesByPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	out := make([]storage.ThreadPluginStateRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.ThreadPluginStateRecord{
			ThreadID:        row.ThreadID,
			PluginID:        row.PluginID,
			View:            row.View,
			StateKey:        row.StateKey,
			DataJSON:        json.RawMessage(row.DataJson),
			UIStateJSON:     json.RawMessage(row.UiStateJson),
			Revision:        row.Revision,
			UpdatedAt:       msToTime(row.UpdatedAtMs),
			ThreadTitle:     row.ThreadTitle.String,
			ThreadStatus:    row.ThreadStatus,
			ThreadUpdatedAt: msToTime(row.ThreadUpdatedAtMs),
			ThreadCwd:       row.ThreadCwd.String,
		})
	}
	return out, nil
}

func (s *Store) SaveThreadPluginData(ctx context.Context, threadID, pluginID, view, stateKey string, data json.RawMessage) error {
	payload := string(data)
	if payload == "" {
		payload = "null"
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return pluginsdb.New(s.db).UpsertThreadPluginData(ctx, pluginsdb.UpsertThreadPluginDataParams{
		ThreadID:    threadID,
		PluginID:    pluginID,
		View:        view,
		StateKey:    stateKey,
		DataJson:    payload,
		UpdatedAtMs: timeToMs(time.Now().UTC()),
	})
}

func (s *Store) SaveThreadPluginUIState(ctx context.Context, threadID, pluginID, view, stateKey string, state json.RawMessage, expectedRevision *int64) error {
	payload := string(state)
	if payload == "" {
		payload = "null"
	}
	nowMs := timeToMs(time.Now().UTC())
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if expectedRevision != nil {
		rows, err := pluginsdb.New(s.db).UpdateThreadPluginUIStateIfMatch(ctx, pluginsdb.UpdateThreadPluginUIStateIfMatchParams{
			UiStateJson:      payload,
			UpdatedAtMs:      nowMs,
			ThreadID:         threadID,
			PluginID:         pluginID,
			View:             view,
			StateKey:         stateKey,
			ExpectedRevision: *expectedRevision,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return storage.ErrPluginStateConflict
		}
		return nil
	}
	return pluginsdb.New(s.db).UpsertThreadPluginUIState(ctx, pluginsdb.UpsertThreadPluginUIStateParams{
		ThreadID:    threadID,
		PluginID:    pluginID,
		View:        view,
		StateKey:    stateKey,
		UiStateJson: payload,
		UpdatedAtMs: nowMs,
	})
}

func pluginSourceFromDB(row pluginsdb.PluginSource) storage.PluginSourceRecord {
	var paths []string
	_ = json.Unmarshal([]byte(row.Paths), &paths)
	return storage.PluginSourceRecord{
		ID:            row.ID,
		Repository:    row.Repository,
		URL:           row.Url,
		Ref:           row.Ref,
		Paths:         paths,
		CommitSHA:     row.CommitSha,
		AutoUpdate:    row.AutoUpdate != 0,
		IsMarketplace: row.IsMarketplace != 0,
		CatalogJSON:   json.RawMessage(row.CatalogJson),
		CheckedAt:     msToTime(row.CheckedAtMs),
		UpdatedAt:     msToTime(row.UpdatedAtMs),
	}
}

func pluginStateFromDB(row pluginsdb.PluginState) storage.PluginStateRecord {
	var config map[string]string
	_ = json.Unmarshal([]byte(row.ConfigJson), &config)
	return storage.PluginStateRecord{
		PluginID:         row.PluginID,
		Enabled:          row.Enabled != 0,
		SourceID:         row.SourceID,
		SourceRepository: row.SourceRepository,
		SourceRef:        row.SourceRef,
		SourcePath:       row.SourcePath,
		CommitSHA:        row.CommitSha,
		RemoteSHA:        row.RemoteSha,
		Config:           config,
		UpdatedAt:        msToTime(row.UpdatedAtMs),
	}
}

func optionalTimeToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}
