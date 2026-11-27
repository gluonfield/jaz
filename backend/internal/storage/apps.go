package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	AppVisibilityPrivate  = "private"
	AppVisibilitySelected = "selected"
	AppVisibilitySpace    = "space"
	AppVisibilityTeam     = "team"

	AppKindArtifact = "artifact"
	AppKindDocument = "document"

	AppStatusActive   = "active"
	AppStatusDeleting = "deleting"

	AppMemberRoleViewer = "viewer"
	AppMemberRoleEditor = "editor"

	AppKVScopePersonal = "personal"
	AppKVScopeShared   = "shared"
	AppKVSharedOwnerID = "shared"
	AppKVLocalOwnerID  = "local"

	DefaultMaxAppKVBytes           int64 = 20 << 20
	DefaultMaxAppAICallsPerUserDay int   = 200
)

var (
	ErrAppNotFound        = errors.New("app not found")
	ErrAppForbidden       = errors.New("forbidden")
	ErrAppConflict        = errors.New("conflict")
	ErrAppKVQuotaExceeded = errors.New("app kv storage quota exceeded")
	ErrAppKVConflict      = errors.New("app kv revision conflict")
	ErrAppDraftConsumed   = errors.New("app draft already consumed or expired")
	ErrAppDraftMismatch   = errors.New("app draft bundle_sha256 mismatch")
)

type AppMember struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
}

type AppMemberInput struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type AppBundleFile struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

type AppExternalURL struct {
	URL         string `json:"url"`
	Host        string `json:"host"`
	Allowlisted bool   `json:"allowlisted"`
	SourceFile  string `json:"source_file,omitempty"`
}

type AppVersion struct {
	AppID        string           `json:"app_id"`
	Version      int              `json:"version"`
	BundleSHA256 string           `json:"bundle_sha256"`
	EntryPath    string           `json:"entry_path"`
	StorageDir   string           `json:"-"`
	SizeBytes    int64            `json:"size_bytes"`
	FileCount    int              `json:"file_count"`
	ManifestJSON json.RawMessage  `json:"manifest,omitempty"`
	Capabilities []string         `json:"capabilities"`
	AllowedTools []string         `json:"allowed_tools"`
	ExternalURLs []AppExternalURL `json:"external_urls"`
	Files        []AppBundleFile  `json:"files,omitempty"`
	CreatedBy    string           `json:"created_by,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
}

type AppDraft struct {
	ID             string           `json:"draft_id"`
	WorkspaceID    string           `json:"-"`
	TargetAppID    string           `json:"target_app_id,omitempty"`
	SourceThreadID string           `json:"source_thread_id,omitempty"`
	Title          string           `json:"title"`
	Description    string           `json:"description,omitempty"`
	IconJSON       json.RawMessage  `json:"icon,omitempty"`
	Kind           string           `json:"kind"`
	BundleSHA256   string           `json:"bundle_sha256"`
	EntryPath      string           `json:"entry_path"`
	StorageDir     string           `json:"-"`
	SizeBytes      int64            `json:"size_bytes"`
	FileCount      int              `json:"file_count"`
	ManifestJSON   json.RawMessage  `json:"manifest,omitempty"`
	Capabilities   []string         `json:"capabilities"`
	AllowedTools   []string         `json:"allowed_tools"`
	ExternalURLs   []AppExternalURL `json:"external_urls"`
	Warnings       []string         `json:"warnings"`
	Files          []AppBundleFile  `json:"files"`
	ExpiresAt      time.Time        `json:"expires_at"`
	ConsumedAt     *time.Time       `json:"consumed_at,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

type App struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"-"`
	OwnerUserID      string          `json:"owner_user_id"`
	OwnerDisplayName string          `json:"owner_display_name,omitempty"`
	OwnerEmail       string          `json:"owner_email,omitempty"`
	SpaceID          *string         `json:"space_id,omitempty"`
	SourceThreadID   string          `json:"source_thread_id,omitempty"`
	Title            string          `json:"title"`
	Description      string          `json:"description,omitempty"`
	IconJSON         json.RawMessage `json:"icon,omitempty"`
	Kind             string          `json:"kind"`
	Visibility       string          `json:"visibility"`
	CurrentVersion   int             `json:"current_version"`
	Status           string          `json:"status"`
	BundleSHA256     string          `json:"bundle_sha256,omitempty"`
	EntryPath        string          `json:"entry_path,omitempty"`
	Capabilities     []string        `json:"capabilities,omitempty"`
	AllowedTools     []string        `json:"allowed_tools,omitempty"`
	IsOwner          bool            `json:"is_owner,omitempty"`
	CanEdit          bool            `json:"can_edit,omitempty"`
	Members          []AppMember     `json:"members,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type AppListFilter struct {
	Scope   string
	SpaceID string
	Kind    string
	Query   string
}

type AppPatch struct {
	Title       *string
	Description *string
	IconJSON    *json.RawMessage
	Visibility  *string
}

type AppKVEntry struct {
	AppID        string    `json:"app_id,omitempty"`
	Scope        string    `json:"scope"`
	ScopeOwnerID string    `json:"-"`
	Key          string    `json:"key"`
	Value        string    `json:"value,omitempty"`
	Revision     int64     `json:"revision"`
	SizeBytes    int64     `json:"size_bytes"`
	UpdatedBy    string    `json:"updated_by,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OrgAppPolicy struct {
	OrgID                  string    `json:"org_id"`
	AllowSharedKV          bool      `json:"allow_shared_kv"`
	AllowAIBridge          bool      `json:"allow_ai_bridge"`
	AllowToolBridge        bool      `json:"allow_tool_bridge"`
	AllowDesktopFullAccess bool      `json:"allow_desktop_full_access"`
	MaxAICallsPerUserDay   int       `json:"max_ai_calls_per_user_day"`
	MaxKVBytesPerApp       int64     `json:"max_kv_bytes_per_app"`
	UpdatedBy              string    `json:"updated_by,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

func DefaultLocalAppPolicy() OrgAppPolicy {
	return OrgAppPolicy{
		OrgID:                  LocalWorkspaceID,
		AllowSharedKV:          false,
		AllowAIBridge:          true,
		AllowToolBridge:        true,
		AllowDesktopFullAccess: true,
		MaxAICallsPerUserDay:   DefaultMaxAppAICallsPerUserDay,
		MaxKVBytesPerApp:       DefaultMaxAppKVBytes,
	}
}

type PublishAppDraftParams struct {
	AppID          string
	IsNew          bool
	WorkspaceID    string
	DraftID        string
	ExpectedSHA256 string
	Title          string
	Description    string
	Visibility     string
	Version        int
	StorageDir     string
}

type AppStore interface {
	CreateAppDraft(ctx context.Context, draft AppDraft) (AppDraft, error)
	GetAppDraft(ctx context.Context, draftID string) (AppDraft, error)
	ReserveNextAppVersion(ctx context.Context, appID string) (exists bool, nextVersion int, err error)
	PublishAppFromDraft(ctx context.Context, params PublishAppDraftParams) (App, AppVersion, error)
	ListApps(ctx context.Context, workspaceID string, filter AppListFilter) ([]App, error)
	GetApp(ctx context.Context, appID string) (App, error)
	GetAppVersion(ctx context.Context, appID string, version int) (AppVersion, error)
	ListAppVersions(ctx context.Context, appID string) ([]AppVersion, error)
	UpdateApp(ctx context.Context, appID string, patch AppPatch) (App, error)
	RollbackAppVersion(ctx context.Context, appID string, targetVersion int) (App, AppVersion, error)
	MarkAppDeleting(ctx context.Context, appID string) ([]string, error)
	FinalizeDeleteApp(ctx context.Context, appID string) error
	ListAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, prefix string) ([]AppKVEntry, int64, error)
	GetAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key string) (AppKVEntry, error)
	PutAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key, value, updatedBy string, expectedRevision *int64, quotaBytes int64) (AppKVEntry, int64, error)
	DeleteAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key string, expectedRevision *int64) (int64, error)
	AssignAppsToBoard(ctx context.Context, boardID string, appIDs []string) error
}
