package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type CloudAccount struct {
	BaseURL          string
	Token            string
	OIDCLoginURL     string
	UserID           string
	OrgID            string
	OrgName          string
	Email            string
	DisplayName      string
	Role             string
	CapabilitiesJSON json.RawMessage
	ConnectedAt      time.Time
	LastSyncAt       time.Time
	LastError        string
}

type CloudSpace struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id,omitempty"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

type CloudThread struct {
	ID                     string          `json:"id"`
	OrgID                  string          `json:"org_id,omitempty"`
	OwnerUserID            string          `json:"owner_user_id,omitempty"`
	SpaceID                string          `json:"space_id,omitempty"`
	RunID                  string          `json:"run_id"`
	Visibility             string          `json:"visibility"`
	SourceType             string          `json:"source_type,omitempty"`
	SourceID               string          `json:"source_id,omitempty"`
	Title                  string          `json:"title,omitempty"`
	Status                 string          `json:"status"`
	TurnEpoch              int64           `json:"turn_epoch,omitempty"`
	ModelProfileID         string          `json:"model_profile_id,omitempty"`
	Model                  string          `json:"model,omitempty"`
	ReasoningEffort        string          `json:"reasoning_effort,omitempty"`
	ExecutorTarget         string          `json:"executor_target,omitempty"`
	RuntimeRefJSON         json.RawMessage `json:"runtime_ref_json,omitempty"`
	PersonalSourcesEnabled bool            `json:"personal_sources_enabled,omitempty"`
	Pinned                 bool            `json:"pinned,omitempty"`
	Archived               bool            `json:"archived,omitempty"`
	Unread                 bool            `json:"unread,omitempty"`
	LastSeq                int64           `json:"last_seq,omitempty"`
	CreatedAt              time.Time       `json:"created_at,omitzero"`
	UpdatedAt              time.Time       `json:"updated_at,omitzero"`
}

func (t CloudThread) ToSession() Session {
	var ref map[string]any
	if len(t.RuntimeRefJSON) > 0 {
		_ = json.Unmarshal(t.RuntimeRefJSON, &ref)
	}
	agentName := "codex"
	if v, ok := ref["agent"].(string); ok && strings.TrimSpace(v) != "" {
		agentName = strings.TrimSpace(v)
	}
	cwd, _ := ref["cwd"].(string)
	updated := t.UpdatedAt
	if updated.IsZero() {
		updated = t.CreatedAt
	}
	created := t.CreatedAt
	if created.IsZero() {
		created = updated
	}
	visibility := "private"
	if t.Visibility == "space" || t.SpaceID != "" {
		visibility = "space"
	}
	runID := t.RunID
	if runID == "" {
		runID = t.ID
	}
	return Session{
		ID:                     t.ID,
		Slug:                   t.ID,
		Title:                  t.Title,
		SourceType:             t.SourceType,
		SourceID:               t.SourceID,
		Status:                 mapCloudStatus(t.Status),
		Runtime:                RuntimeACP,
		RuntimeRef:             &RuntimeRef{Type: RuntimeACP, Agent: agentName, Cwd: cwd},
		ModelProvider:          t.ModelProfileID,
		Model:                  t.Model,
		ReasoningEffort:        t.ReasoningEffort,
		SpaceID:                t.SpaceID,
		Visibility:             visibility,
		RunID:                  runID,
		PersonalSourcesEnabled: t.PersonalSourcesEnabled,
		Backend:                "cloud",
		Pinned:                 t.Pinned,
		Archived:               t.Archived,
		Unread:                 t.Unread,
		CreatedAt:              created,
		UpdatedAt:              updated,
		LastAttentionAt:        updated,
	}
}

func mapCloudStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running", "queued":
		return StatusRunning
	case "error", "failed":
		return StatusError
	case "interrupted":
		return StatusInterrupted
	default:
		return StatusIdle
	}
}

type CloudThreadEvent struct {
	ThreadID    string          `json:"thread_id"`
	Seq         int64           `json:"seq"`
	OrgID       string          `json:"org_id,omitempty"`
	TurnEpoch   int64           `json:"turn_epoch,omitempty"`
	ActorUserID string          `json:"actor_user_id,omitempty"`
	Type        string          `json:"type"`
	PayloadJSON json.RawMessage `json:"payload,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type CloudInstruction struct {
	OrgID     string    `json:"org_id"`
	Scope     string    `json:"scope"`
	ScopeID   string    `json:"scope_id"`
	Kind      string    `json:"kind"`
	BodyMD    string    `json:"body_md"`
	Version   string    `json:"version"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

type CloudUsageEvent struct {
	ID                int64     `json:"id"`
	OrgID             string    `json:"org_id"`
	UserID            string    `json:"user_id"`
	ThreadID          string    `json:"thread_id"`
	ModelProfileID    string    `json:"model_profile_id"`
	Model             string    `json:"model"`
	InputTokens       int64     `json:"input_tokens"`
	CachedInputTokens int64     `json:"cached_input_tokens"`
	OutputTokens      int64     `json:"output_tokens"`
	CostUSD           float64   `json:"cost_usd"`
	CreatedAt         time.Time `json:"created_at"`
}

type CloudThreadUserStatePatch struct {
	Pinned                 *bool
	Archived               *bool
	PersonalSourcesEnabled *bool
	Unread                 *bool
}

type CloudMirrorStore interface {
	LoadCloudAccount(context.Context) (CloudAccount, bool, error)
	SaveCloudAccount(context.Context, CloudAccount) error
	UpdateCloudSyncState(context.Context, time.Time, string) error
	ClearCloudMirror(context.Context) error

	ReplaceCloudSpaces(context.Context, []CloudSpace) error
	UpsertCloudSpace(context.Context, CloudSpace) error
	ListCloudSpaces(context.Context) ([]CloudSpace, error)

	SyncCloudThreads(context.Context, []CloudThread) error
	UpsertCloudThread(context.Context, CloudThread) error
	GetCloudThread(context.Context, string) (CloudThread, bool, error)
	ListCloudThreads(context.Context, bool) ([]CloudThread, error)
	UpdateCloudThreadUserState(context.Context, string, CloudThreadUserStatePatch) (CloudThread, error)
	UpdateCloudThreadStatus(context.Context, string, string, time.Time) error

	AppendCloudThreadEvents(context.Context, string, []CloudThreadEvent) error
	ListCloudThreadEvents(context.Context, string, int64) ([]CloudThreadEvent, error)

	UpsertCloudInstruction(context.Context, CloudInstruction) error
	GetCloudInstruction(context.Context, string, string, string, string) (CloudInstruction, bool, error)

	UpsertCloudUsageEvents(context.Context, []CloudUsageEvent) error
	MaxCloudUsageEventID(context.Context) (int64, error)
}
