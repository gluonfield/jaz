package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type CloudAccount struct {
	ID               string
	BaseURL          string
	Token            string
	RefreshToken     string
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

type CloudSpaceMember struct {
	SpaceID     string `json:"space_id,omitempty"`
	UserID      string `json:"user_id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role,omitempty"`
}

type CloudSpace struct {
	AccountID        string             `json:"-"`
	ID               string             `json:"id"`
	OrgID            string             `json:"org_id,omitempty"`
	Name             string             `json:"name"`
	Visibility       string             `json:"visibility,omitempty"`
	CreatedBy        string             `json:"created_by,omitempty"`
	DefaultGitRepo   string             `json:"default_git_repo,omitempty"`
	DefaultGitBranch string             `json:"default_git_branch,omitempty"`
	CanEdit          bool               `json:"can_edit,omitempty"`
	MemberCount      int                `json:"member_count,omitempty"`
	SourceCount      int                `json:"source_count,omitempty"`
	Members          []CloudSpaceMember `json:"members,omitempty"`
	CreatedAt        time.Time          `json:"created_at,omitzero"`
	UpdatedAt        time.Time          `json:"updated_at,omitzero"`
}

type CloudThread struct {
	AccountID              string          `json:"-"`
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

func NormalizeWorkspaceID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return LocalWorkspaceID
	}
	return trimmed
}

func (t CloudThread) ToSession() Session {
	var ref map[string]any
	if len(t.RuntimeRefJSON) > 0 {
		_ = json.Unmarshal(t.RuntimeRefJSON, &ref)
	}
	var agentName string
	if v, ok := ref["agent"].(string); ok && strings.TrimSpace(v) != "" {
		agentName = strings.TrimSpace(v)
	}
	if agentName == "" {
		agentName = inferCloudThreadAgent(t.ModelProfileID, t.Model)
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
	if t.Visibility == "space" {
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
		WorkspaceID:            t.AccountID,
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

func inferCloudThreadAgent(modelProfileID, model string) string {
	profile := strings.ToLower(strings.TrimSpace(modelProfileID))
	if strings.HasPrefix(profile, "acp:") {
		if agent := strings.TrimSpace(strings.TrimPrefix(profile, "acp:")); agent != "" {
			return agent
		}
	}
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "gemini") || strings.HasPrefix(m, "google/") || strings.Contains(profile, "vertex"):
		return "gemini"
	case strings.Contains(m, "claude") || strings.Contains(profile, "anthropic"):
		return "claude"
	case strings.Contains(m, "grok") || strings.Contains(profile, "xai"):
		return "grok"
	case strings.Contains(m, "kimi") || strings.Contains(profile, "moonshot"):
		return "kimi"
	case strings.Contains(m, "gpt") || strings.Contains(m, "codex") || strings.Contains(profile, "openai"):
		return "codex"
	default:
		return ""
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
	ListCloudAccounts(context.Context) ([]CloudAccount, error)
	LoadCloudAccount(ctx context.Context, id string) (CloudAccount, bool, error)
	SaveCloudAccount(context.Context, CloudAccount) error
	UpdateCloudSyncState(ctx context.Context, accountID string, lastSyncAt time.Time, lastError string) error
	DeleteCloudAccount(ctx context.Context, accountID string) error

	LoadActiveWorkspaceID(context.Context) (string, error)
	SaveActiveWorkspaceID(context.Context, string) error

	ReplaceCloudSpaces(ctx context.Context, accountID string, spaces []CloudSpace) error
	UpsertCloudSpace(ctx context.Context, accountID string, space CloudSpace) error
	ListCloudSpaces(ctx context.Context, accountID string) ([]CloudSpace, error)

	SyncCloudThreads(ctx context.Context, accountID string, threads []CloudThread) error
	UpsertCloudThread(ctx context.Context, accountID string, thread CloudThread) error
	GetCloudThread(ctx context.Context, accountID, id string) (CloudThread, bool, error)
	ListCloudThreads(ctx context.Context, accountID string, archived bool) ([]CloudThread, error)
	UpdateCloudThreadUserState(ctx context.Context, accountID, id string, patch CloudThreadUserStatePatch) (CloudThread, error)
	UpdateCloudThreadStatus(ctx context.Context, accountID, id, status string, updatedAt time.Time) error
	DeleteCloudThread(ctx context.Context, accountID, id string) error

	AppendCloudThreadEvents(ctx context.Context, accountID, threadID string, events []CloudThreadEvent) error
	ListCloudThreadEvents(ctx context.Context, accountID, threadID string, afterSeq int64) ([]CloudThreadEvent, error)

	UpsertCloudInstruction(ctx context.Context, accountID string, doc CloudInstruction) error
	GetCloudInstruction(ctx context.Context, accountID, orgID, scope, scopeID, kind string) (CloudInstruction, bool, error)

	UpsertCloudUsageEvents(ctx context.Context, accountID string, events []CloudUsageEvent) error
	MaxCloudUsageEventID(ctx context.Context, accountID string) (int64, error)
}
