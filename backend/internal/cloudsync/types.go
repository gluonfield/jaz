package cloudsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const LocalWorkspaceID = storage.LocalWorkspaceID

var (
	ErrNotConnected = errors.New("not signed in to Jaz Cloud")
	ErrInvalidURL   = errors.New("Jaz Cloud connections require HTTPS")
	ErrInvalidPath  = errors.New("invalid cloud resource path")
	ErrNotFound     = errors.New("workspace not found")
)

type CloudUser struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id,omitempty"`
	OrgID       string          `json:"org_id"`
	OrgName     string          `json:"org_name,omitempty"`
	OrgIcon     json.RawMessage `json:"org_icon,omitempty"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        string          `json:"role"`
}

type WorkspaceSummary struct {
	ID           string          `json:"id"`
	URL          string          `json:"url"`
	OrgID        string          `json:"org_id,omitempty"`
	OrgName      string          `json:"org_name,omitempty"`
	Email        string          `json:"email,omitempty"`
	DisplayName  string          `json:"display_name,omitempty"`
	Role         string          `json:"role,omitempty"`
	OIDCLoginURL string          `json:"oidc_login_url,omitempty"`
	Capabilities json.RawMessage `json:"capabilities,omitempty"`
	ConnectedAt  time.Time       `json:"connected_at,omitzero"`
	LastSyncAt   time.Time       `json:"last_sync_at,omitzero"`
	LastError    string          `json:"last_error,omitempty"`
}

type Status struct {
	ActiveWorkspaceID string             `json:"active_workspace_id"`
	Connected         bool               `json:"connected"`
	Status            string             `json:"status"` // "connected" when active workspace is Cloud, else "disconnected"
	URL               string             `json:"url,omitempty"`
	User              *CloudUser         `json:"user,omitempty"`
	OIDCLoginURL      string             `json:"oidc_login_url,omitempty"`
	Capabilities      json.RawMessage    `json:"capabilities,omitempty"`
	ConnectedAt       time.Time          `json:"connected_at,omitzero"`
	LastSyncAt        time.Time          `json:"last_sync_at,omitzero"`
	LastError         string             `json:"last_error,omitempty"`
	Workspaces        []WorkspaceSummary `json:"workspaces"`
}

type ConnectRequest struct {
	URL          string `json:"url"`
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type ThreadGitRepoSpec struct {
	storage.CloudGitRepoSpec
	Branch string `json:"branch,omitempty"`
}

type CreateThreadRequest struct {
	WorkspaceID     string              `json:"-"`
	Title           string              `json:"title,omitempty"`
	SourceType      string              `json:"source_type,omitempty"`
	SourceID        string              `json:"source_id,omitempty"`
	SpaceID         string              `json:"space_id,omitempty"`
	ModelProfileID  string              `json:"model_profile_id,omitempty"`
	Model           string              `json:"model,omitempty"`
	ReasoningEffort string              `json:"reasoning_effort,omitempty"`
	ExecutorTarget  string              `json:"executor_target,omitempty"`
	GitRepo         string              `json:"git_repo,omitempty"`
	GitBaseBranch   string              `json:"git_base_branch,omitempty"`
	GitBranch       string              `json:"git_branch,omitempty"`
	GitRepos        []ThreadGitRepoSpec `json:"git_repos,omitzero"`
}

type ProjectedTranscript struct {
	Session         storage.Session
	Messages        []storage.Message
	Events          []sessionevents.Event
	LatestSeq       int64
	HistoryRevision int64
	ACPState        string
	ACPAssistant    string
	ACPError        string
}

type UpstreamError struct {
	StatusCode int
	Message    string
	Body       json.RawMessage
}

func (e *UpstreamError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("cloud request failed (%d)", e.StatusCode)
}

type wireThread struct {
	ID                     string          `json:"id"`
	OrgID                  string          `json:"org_id"`
	OwnerUserID            string          `json:"owner_user_id"`
	SpaceID                *string         `json:"space_id"`
	RunID                  string          `json:"run_id"`
	Visibility             string          `json:"visibility"`
	SourceType             string          `json:"source_type"`
	SourceID               string          `json:"source_id"`
	Title                  string          `json:"title"`
	Status                 string          `json:"status"`
	TurnEpoch              int64           `json:"turn_epoch"`
	ModelProfileID         string          `json:"model_profile_id"`
	Model                  string          `json:"model"`
	ReasoningEffort        string          `json:"reasoning_effort"`
	ExecutorTarget         string          `json:"executor_target"`
	RuntimeRefJSON         json.RawMessage `json:"runtime_ref_json"`
	PersonalSourcesEnabled bool            `json:"personal_sources_enabled"`
	Pinned                 bool            `json:"pinned"`
	Archived               bool            `json:"archived"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

func (w wireThread) toStorage() storage.CloudThread {
	var spaceID string
	if w.SpaceID != nil {
		spaceID = *w.SpaceID
	}
	return storage.CloudThread{
		ID:                     w.ID,
		OrgID:                  w.OrgID,
		OwnerUserID:            w.OwnerUserID,
		SpaceID:                spaceID,
		RunID:                  w.RunID,
		Visibility:             w.Visibility,
		SourceType:             w.SourceType,
		SourceID:               w.SourceID,
		Title:                  w.Title,
		Status:                 w.Status,
		TurnEpoch:              w.TurnEpoch,
		ModelProfileID:         w.ModelProfileID,
		Model:                  w.Model,
		ReasoningEffort:        w.ReasoningEffort,
		ExecutorTarget:         w.ExecutorTarget,
		RuntimeRefJSON:         w.RuntimeRefJSON,
		PersonalSourcesEnabled: w.PersonalSourcesEnabled,
		Pinned:                 w.Pinned,
		Archived:               w.Archived,
		CreatedAt:              w.CreatedAt,
		UpdatedAt:              w.UpdatedAt,
	}
}

type wireEvent struct {
	ThreadID    string          `json:"thread_id"`
	Seq         int64           `json:"seq"`
	OrgID       string          `json:"org_id"`
	TurnEpoch   int64           `json:"turn_epoch"`
	ActorUserID *string         `json:"actor_user_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (w wireEvent) toStorage(fallbackThreadID string) storage.CloudThreadEvent {
	threadID := w.ThreadID
	if threadID == "" {
		threadID = fallbackThreadID
	}
	var actor string
	if w.ActorUserID != nil {
		actor = *w.ActorUserID
	}
	created := w.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return storage.CloudThreadEvent{
		ThreadID:    threadID,
		Seq:         w.Seq,
		OrgID:       w.OrgID,
		TurnEpoch:   w.TurnEpoch,
		ActorUserID: actor,
		Type:        w.Type,
		PayloadJSON: w.Payload,
		CreatedAt:   created,
	}
}
