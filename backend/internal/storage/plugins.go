package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrPluginSourceNotFound = errors.New("plugin source not found")
	ErrPluginStateConflict  = errors.New("plugin view state revision conflict")
)

type PluginSourceRecord struct {
	WorkspaceID   string          `json:"workspace_id,omitempty"`
	ID            string          `json:"id"`
	Repository    string          `json:"repository"`
	URL           string          `json:"url"`
	Ref           string          `json:"ref,omitempty"`
	Paths         []string        `json:"paths,omitempty"`
	CommitSHA     string          `json:"commit_sha,omitempty"`
	AutoUpdate    bool            `json:"auto_update,omitempty"`
	IsMarketplace bool            `json:"is_marketplace,omitempty"`
	CatalogJSON   json.RawMessage `json:"catalog_json,omitempty"`
	CheckedAt     time.Time       `json:"checked_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
}

type PluginStateRecord struct {
	WorkspaceID      string            `json:"workspace_id,omitempty"`
	PluginID         string            `json:"plugin_id"`
	Enabled          bool              `json:"enabled"`
	SourceID         string            `json:"source_id,omitempty"`
	SourceRepository string            `json:"source_repository,omitempty"`
	SourceRef        string            `json:"source_ref,omitempty"`
	SourcePath       string            `json:"source_path,omitempty"`
	CommitSHA        string            `json:"commit_sha,omitempty"`
	RemoteSHA        string            `json:"remote_sha,omitempty"`
	Config           map[string]string `json:"config,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at,omitempty"`
}

type ThreadPluginStateRecord struct {
	ThreadID        string          `json:"thread_id"`
	PluginID        string          `json:"plugin_id"`
	View            string          `json:"view"`
	StateKey        string          `json:"state_key"`
	DataJSON        json.RawMessage `json:"data_json"`
	UIStateJSON     json.RawMessage `json:"ui_state_json"`
	Revision        int64           `json:"revision"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ThreadTitle     string          `json:"thread_title,omitempty"`
	ThreadStatus    string          `json:"thread_status,omitempty"`
	ThreadUpdatedAt time.Time       `json:"thread_updated_at,omitempty"`
	ThreadCwd       string          `json:"thread_cwd,omitempty"`
}

type PluginStore interface {
	ListPluginSources(ctx context.Context, workspaceID string) ([]PluginSourceRecord, error)
	GetPluginSource(ctx context.Context, workspaceID, id string) (PluginSourceRecord, bool, error)
	SavePluginSource(context.Context, PluginSourceRecord) error
	DeletePluginSource(ctx context.Context, workspaceID, id string) (bool, error)

	ListPluginStates(ctx context.Context, workspaceID string) ([]PluginStateRecord, error)
	GetPluginState(ctx context.Context, workspaceID, pluginID string) (PluginStateRecord, bool, error)
	SavePluginState(context.Context, PluginStateRecord) error
	DeletePluginState(ctx context.Context, workspaceID, pluginID string) error

	GetThreadPluginState(ctx context.Context, threadID, pluginID, view, stateKey string) (ThreadPluginStateRecord, bool, error)
	ListThreadPluginStatesByPlugin(ctx context.Context, workspaceID, pluginID string) ([]ThreadPluginStateRecord, error)
	SaveThreadPluginData(ctx context.Context, threadID, pluginID, view, stateKey string, data json.RawMessage) error
	SaveThreadPluginUIState(ctx context.Context, threadID, pluginID, view, stateKey string, state json.RawMessage, expectedRevision *int64) error
}
