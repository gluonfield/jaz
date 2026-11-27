package apps

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/appbundle"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	appIDLength         = 26
	appIDAlphabet       = "abcdefghijklmnopqrstuvwxyz0123456789"
	appDraftTTL         = time.Hour
	MaxKVValueBytes     = 2 << 20
	MaxAIPromptRunes    = 64000
	MaxAISystemRunes    = 16000
	DefaultAIMaxTokens  = 2048
	MaxAIMaxTokens      = 8192
)

var (
	validAppIDRe = regexp.MustCompile(`^[a-z0-9]{26}$`)
	sha256HexRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)

	ErrSharedKVDisabled = errors.New("shared app storage requires a Jaz Cloud workspace")
	ErrCapabilityDenied = errors.New("app does not declare required capability")
	ErrToolNotAllowed   = errors.New("tool is not in app's explicit allowed_tools list")
	ErrAIUnavailable    = errors.New("no local model provider configured for AI completions")
)

type ToolBridge interface {
	CallAppBridgeTool(ctx context.Context, name string, arguments json.RawMessage) (*mcpsdk.CallToolResult, error)
}

type SessionCreator interface {
	LoadSession(ref string) (storage.Session, error)
	CreateSession(input storage.CreateSession) (storage.Session, error)
}

type Service struct {
	store      storage.AppStore
	sessions   SessionCreator
	provider   provider.Provider
	toolBridge ToolBridge
	appsRoot   string
	workspace  string

	keyOnce sync.Once
	key     []byte
	keyErr  error
}

func NewService(
	store storage.AppStore,
	sessions SessionCreator,
	modelProvider provider.Provider,
	toolBridge ToolBridge,
	rootDir string,
	workspaceDir string,
) *Service {
	return &Service{
		store:      store,
		sessions:   sessions,
		provider:   modelProvider,
		toolBridge: toolBridge,
		appsRoot:   filepath.Join(rootDir, "apps"),
		workspace:  workspaceDir,
	}
}

func IsValidAppID(id string) bool {
	return validAppIDRe.MatchString(id)
}

func IsValidSHA256(s string) bool {
	return sha256HexRe.MatchString(s)
}

func NormalizeVisibility(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", storage.AppVisibilityPrivate:
		return storage.AppVisibilityPrivate, nil
	case storage.AppVisibilitySelected:
		return storage.AppVisibilitySelected, nil
	case storage.AppVisibilitySpace:
		return storage.AppVisibilitySpace, nil
	case storage.AppVisibilityTeam:
		return storage.AppVisibilityTeam, nil
	default:
		return "", errors.New("visibility must be private, selected, space, or team (public links are disabled)")
	}
}

func generateAppID() (string, error) {
	out := make([]byte, appIDLength)
	max := big.NewInt(int64(len(appIDAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = appIDAlphabet[n.Int64()]
	}
	return string(out), nil
}

func (s *Service) signingKey() ([]byte, error) {
	s.keyOnce.Do(func() {
		s.key, s.keyErr = loadOrCreateTicketKey(s.appsRoot)
	})
	return s.key, s.keyErr
}

func writeBundleDir(dir string, files []appbundle.FileEntry) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, f := range files {
		cleanPath, err := appbundle.ValidatePath(f.Path)
		if err != nil {
			return err
		}
		fullPath := filepath.Join(dir, filepath.FromSlash(cleanPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, f.Data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func copyBundleFiles(srcDir, dstDir string, files []storage.AppBundleFile) error {
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return err
	}
	for _, f := range files {
		cleanPath, err := appbundle.ValidatePath(f.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(srcDir, filepath.FromSlash(cleanPath)))
		if err != nil {
			return fmt.Errorf("read staged file %s: %w", cleanPath, err)
		}
		dstPath := filepath.Join(dstDir, filepath.FromSlash(cleanPath))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, 0o600); err != nil {
			return fmt.Errorf("write app file %s: %w", cleanPath, err)
		}
	}
	return nil
}

func (s *Service) StageDraft(ctx context.Context, bundle appbundle.Bundle, targetAppID, sourceThreadID string) (storage.AppDraft, error) {
	targetAppID = strings.TrimSpace(targetAppID)
	if targetAppID != "" {
		if !IsValidAppID(targetAppID) {
			return storage.AppDraft{}, errors.New("invalid target_app_id")
		}
		if _, _, err := s.store.ReserveNextAppVersion(ctx, targetAppID); err != nil {
			return storage.AppDraft{}, err
		}
	}
	draftID := "adraft_" + uuid.NewString()
	draftDir := filepath.Join(s.appsRoot, "drafts", draftID)
	if err := writeBundleDir(draftDir, bundle.Files); err != nil {
		_ = os.RemoveAll(draftDir)
		return storage.AppDraft{}, err
	}

	filesMeta := make([]storage.AppBundleFile, 0, len(bundle.Files))
	for _, f := range bundle.Files {
		filesMeta = append(filesMeta, storage.AppBundleFile{
			Path:        f.Path,
			SizeBytes:   f.SizeBytes,
			SHA256:      f.SHA256,
			ContentType: f.ContentType,
		})
	}
	extURLs := make([]storage.AppExternalURL, 0, len(bundle.ExternalURLs))
	for _, u := range bundle.ExternalURLs {
		extURLs = append(extURLs, storage.AppExternalURL{
			URL:         u.URL,
			Host:        u.Host,
			Allowlisted: u.Allowlisted,
			SourceFile:  u.SourceFile,
		})
	}
	var iconJSON json.RawMessage
	if bundle.Manifest.Icon != nil {
		iconJSON, _ = json.Marshal(bundle.Manifest.Icon)
	}

	draft, err := s.store.CreateAppDraft(ctx, storage.AppDraft{
		ID:             draftID,
		WorkspaceID:    storage.LocalWorkspaceID,
		TargetAppID:    targetAppID,
		SourceThreadID: strings.TrimSpace(sourceThreadID),
		Title:          bundle.Manifest.Title,
		Description:    bundle.Manifest.Description,
		IconJSON:       iconJSON,
		Kind:           bundle.Manifest.Kind,
		BundleSHA256:   bundle.BundleSHA256,
		EntryPath:      bundle.Manifest.Entry,
		StorageDir:     draftDir,
		SizeBytes:      bundle.SizeBytes,
		FileCount:      bundle.FileCount,
		ManifestJSON:   bundle.ManifestJSON,
		Capabilities:   bundle.Manifest.Capabilities,
		AllowedTools:   bundle.Manifest.AllowedTools,
		ExternalURLs:   extURLs,
		Warnings:       bundle.Warnings,
		Files:          filesMeta,
		ExpiresAt:      time.Now().UTC().Add(appDraftTTL),
	})
	if err != nil {
		_ = os.RemoveAll(draftDir)
		return storage.AppDraft{}, err
	}
	return draft, nil
}

type PublishInput struct {
	RequestedID  string
	DraftID      string
	BundleSHA256 string
	Title        string
	Description  string
	Visibility   string
}

func (s *Service) PublishFromDraft(ctx context.Context, input PublishInput) (storage.App, storage.AppVersion, bool, error) {
	draftID := strings.TrimSpace(input.DraftID)
	sha := strings.ToLower(strings.TrimSpace(input.BundleSHA256))
	if draftID == "" || !IsValidSHA256(sha) {
		return storage.App{}, storage.AppVersion{}, false, errors.New("draft_id and 64-hex bundle_sha256 are required (stage a preflight draft first)")
	}
	if title := strings.TrimSpace(input.Title); title != "" && utf8.RuneCountInString(title) > appbundle.MaxTitleRunes {
		return storage.App{}, storage.AppVersion{}, false, errors.New("title exceeds maximum length")
	}
	if desc := strings.TrimSpace(input.Description); utf8.RuneCountInString(desc) > appbundle.MaxDescRunes {
		return storage.App{}, storage.AppVersion{}, false, errors.New("description exceeds maximum length")
	}

	var vis string
	if strings.TrimSpace(input.Visibility) != "" {
		var err error
		vis, err = NormalizeVisibility(input.Visibility)
		if err != nil {
			return storage.App{}, storage.AppVersion{}, false, err
		}
	}

	draft, err := s.store.GetAppDraft(ctx, draftID)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, false, err
	}
	if draft.ConsumedAt != nil || time.Now().UTC().After(draft.ExpiresAt) {
		return storage.App{}, storage.AppVersion{}, false, storage.ErrAppDraftConsumed
	}
	if draft.BundleSHA256 != sha {
		return storage.App{}, storage.AppVersion{}, false, storage.ErrAppDraftMismatch
	}

	var (
		appID       string
		isNew       bool
		nextVersion int
	)
	requestedID := strings.TrimSpace(input.RequestedID)
	if requestedID == "" || requestedID == "new" {
		if draft.TargetAppID != "" {
			return storage.App{}, storage.AppVersion{}, false, fmt.Errorf("draft is bound to target_app_id %s", draft.TargetAppID)
		}
		appID, err = generateAppID()
		if err != nil {
			return storage.App{}, storage.AppVersion{}, false, err
		}
		isNew = true
		nextVersion = 1
	} else {
		if !IsValidAppID(requestedID) {
			return storage.App{}, storage.AppVersion{}, false, errors.New("app id must be 26 lowercase alphanumeric characters or 'new'")
		}
		if draft.TargetAppID != "" && draft.TargetAppID != requestedID {
			return storage.App{}, storage.AppVersion{}, false, storage.ErrAppForbidden
		}
		exists, nv, err := s.store.ReserveNextAppVersion(ctx, requestedID)
		if err != nil {
			return storage.App{}, storage.AppVersion{}, false, err
		}
		appID = requestedID
		isNew = !exists
		nextVersion = nv
	}

	dstDir := filepath.Join(s.appsRoot, appID, fmt.Sprintf("v%d", nextVersion))
	if err := copyBundleFiles(draft.StorageDir, dstDir, draft.Files); err != nil {
		_ = os.RemoveAll(dstDir)
		return storage.App{}, storage.AppVersion{}, false, err
	}

	app, ver, err := s.store.PublishAppFromDraft(ctx, storage.PublishAppDraftParams{
		AppID:          appID,
		IsNew:          isNew,
		WorkspaceID:    storage.LocalWorkspaceID,
		DraftID:        draft.ID,
		ExpectedSHA256: sha,
		Title:          input.Title,
		Description:    input.Description,
		Visibility:     vis,
		Version:        nextVersion,
		StorageDir:     dstDir,
	})
	if err != nil {
		_ = os.RemoveAll(dstDir)
		return storage.App{}, storage.AppVersion{}, false, err
	}
	_ = os.RemoveAll(draft.StorageDir)
	return app, ver, isNew, nil
}

func (s *Service) ListApps(ctx context.Context, filter storage.AppListFilter) ([]storage.App, error) {
	return s.store.ListApps(ctx, storage.LocalWorkspaceID, filter)
}

func (s *Service) GetApp(ctx context.Context, appID string) (storage.App, *storage.AppVersion, error) {
	app, err := s.store.GetApp(ctx, appID)
	if err != nil {
		return storage.App{}, nil, err
	}
	var currentVer *storage.AppVersion
	if app.CurrentVersion > 0 {
		if v, err := s.store.GetAppVersion(ctx, app.ID, app.CurrentVersion); err == nil {
			currentVer = &v
		}
	}
	return app, currentVer, nil
}

func (s *Service) ListVersions(ctx context.Context, appID string) ([]storage.AppVersion, error) {
	if _, err := s.store.GetApp(ctx, appID); err != nil {
		return nil, err
	}
	return s.store.ListAppVersions(ctx, appID)
}

func (s *Service) UpdateApp(ctx context.Context, appID string, patch storage.AppPatch) (storage.App, error) {
	return s.store.UpdateApp(ctx, appID, patch)
}

func (s *Service) Rollback(ctx context.Context, appID string, version int) (storage.App, storage.AppVersion, error) {
	return s.store.RollbackAppVersion(ctx, appID, version)
}

func (s *Service) DeleteApp(ctx context.Context, appID string) error {
	dirs, err := s.store.MarkAppDeleting(ctx, appID)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(s.appsRoot, appID))
	for _, dir := range dirs {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	return s.store.FinalizeDeleteApp(ctx, appID)
}

type RemixResult struct {
	Thread        storage.Session `json:"thread"`
	RunID         string          `json:"run_id"`
	SourceAppID   string          `json:"source_app_id"`
	SourceVersion int             `json:"source_version"`
	BundleSHA256  string          `json:"bundle_sha256"`
	HydratedFiles []string        `json:"hydrated_files"`
}

func (s *Service) RemixApp(ctx context.Context, appID, title string) (RemixResult, error) {
	if s.sessions == nil {
		return RemixResult{}, errors.New("sessions store not configured")
	}
	app, err := s.store.GetApp(ctx, appID)
	if err != nil {
		return RemixResult{}, err
	}
	if app.CurrentVersion <= 0 {
		return RemixResult{}, errors.New("app has no published version to remix")
	}
	ver, err := s.store.GetAppVersion(ctx, app.ID, app.CurrentVersion)
	if err != nil {
		return RemixResult{}, err
	}

	remixDir := filepath.Join(s.workspace, "remixes", fmt.Sprintf("%s-%d", app.ID[:8], time.Now().UTC().Unix()))
	if err := copyBundleFiles(ver.StorageDir, remixDir, ver.Files); err != nil {
		_ = os.RemoveAll(remixDir)
		return RemixResult{}, err
	}

	threadTitle := strings.TrimSpace(title)
	if threadTitle == "" {
		threadTitle = "Remix: " + app.Title
	}
	session, err := s.sessions.CreateSession(storage.CreateSession{
		Title:       threadTitle,
		Runtime:     storage.RuntimeACP,
		RuntimeRef:  &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: remixDir},
		WorkspaceID: storage.LocalWorkspaceID,
		SourceType:  "app_remix",
		SourceID:    app.ID,
	})
	if err != nil {
		_ = os.RemoveAll(remixDir)
		return RemixResult{}, err
	}
	paths := make([]string, 0, len(ver.Files))
	for _, f := range ver.Files {
		paths = append(paths, f.Path)
	}
	return RemixResult{
		Thread:        session,
		RunID:         session.ID,
		SourceAppID:   app.ID,
		SourceVersion: ver.Version,
		BundleSHA256:  ver.BundleSHA256,
		HydratedFiles: paths,
	}, nil
}

type TicketResponse struct {
	Ticket        string `json:"ticket"`
	ExpiresAt     string `json:"expires_at"`
	AppID         string `json:"app_id"`
	Version       int    `json:"version"`
	BundleSHA256  string `json:"bundle_sha256"`
	EntryPath     string `json:"entry_path"`
	ContentOrigin string `json:"content_origin"`
	ContentURL    string `json:"content_url"`
}

func (s *Service) IssueTicket(ctx context.Context, appID string, requestedVersion int, baseURL string) (TicketResponse, error) {
	app, err := s.store.GetApp(ctx, appID)
	if err != nil {
		return TicketResponse{}, err
	}
	version := app.CurrentVersion
	if requestedVersion > 0 {
		version = requestedVersion
	}
	if version <= 0 {
		return TicketResponse{}, storage.ErrAppNotFound
	}
	ver, err := s.store.GetAppVersion(ctx, app.ID, version)
	if err != nil {
		return TicketResponse{}, err
	}
	key, err := s.signingKey()
	if err != nil {
		return TicketResponse{}, err
	}
	now := time.Now().UTC()
	exp := now.Add(ticketTTL)
	tok, err := signHS256(key, TicketClaims{
		AppID:        app.ID,
		Version:      ver.Version,
		BundleSHA256: ver.BundleSHA256,
		Scope:        ticketScope,
		Iss:          ticketIssuer,
		Aud:          ticketAudience,
		Iat:          now.Unix(),
		Nbf:          now.Unix(),
		Exp:          exp.Unix(),
	})
	if err != nil {
		return TicketResponse{}, err
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	contentURL := fmt.Sprintf("%s/app-content/%s/%s?ticket=%s", base, app.ID, ver.EntryPath, tok)
	return TicketResponse{
		Ticket:        tok,
		ExpiresAt:     exp.Format(time.RFC3339),
		AppID:         app.ID,
		Version:       ver.Version,
		BundleSHA256:  ver.BundleSHA256,
		EntryPath:     ver.EntryPath,
		ContentOrigin: base,
		ContentURL:    contentURL,
	}, nil
}

func (s *Service) ParseTicket(token string) (TicketClaims, error) {
	key, err := s.signingKey()
	if err != nil {
		return TicketClaims{}, err
	}
	return verifyHS256(key, token)
}

type BundleFileRead struct {
	Data         []byte
	ContentType  string
	BundleSHA256 string
}

func (s *Service) ReadBundleFile(ctx context.Context, claims TicketClaims, rawRelpath string) (BundleFileRead, error) {
	app, err := s.store.GetApp(ctx, claims.AppID)
	if err != nil {
		return BundleFileRead{}, err
	}
	ver, err := s.store.GetAppVersion(ctx, app.ID, claims.Version)
	if err != nil || ver.BundleSHA256 != claims.BundleSHA256 {
		return BundleFileRead{}, storage.ErrAppNotFound
	}
	relpath := strings.TrimPrefix(rawRelpath, "/")
	if relpath == "" {
		relpath = ver.EntryPath
	}
	cleanPath, err := appbundle.ValidatePath(relpath)
	if err != nil {
		return BundleFileRead{}, err
	}
	var matched *storage.AppBundleFile
	for i := range ver.Files {
		if ver.Files[i].Path == cleanPath {
			matched = &ver.Files[i]
			break
		}
	}
	if matched == nil {
		return BundleFileRead{}, storage.ErrAppNotFound
	}
	data, err := os.ReadFile(filepath.Join(ver.StorageDir, filepath.FromSlash(matched.Path)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BundleFileRead{}, storage.ErrAppNotFound
		}
		return BundleFileRead{}, err
	}
	ct := matched.ContentType
	if ct == "" {
		ct = appbundle.ResolveContentType(matched.Path)
	}
	return BundleFileRead{
		Data:         data,
		ContentType:  ct,
		BundleSHA256: ver.BundleSHA256,
	}, nil
}

func (s *Service) authorizeCapability(ctx context.Context, appID, capName string) (storage.App, storage.AppVersion, error) {
	app, err := s.store.GetApp(ctx, appID)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	if app.CurrentVersion <= 0 {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppNotFound
	}
	ver, err := s.store.GetAppVersion(ctx, app.ID, app.CurrentVersion)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	if capName != "" && !slices.Contains(ver.Capabilities, capName) {
		return storage.App{}, storage.AppVersion{}, fmt.Errorf("%w: %s", ErrCapabilityDenied, capName)
	}
	return app, ver, nil
}

func resolveLocalKVScope(rawScope string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(rawScope)) {
	case "", storage.AppKVScopePersonal:
		return storage.AppKVScopePersonal, storage.AppKVLocalOwnerID, nil
	case storage.AppKVScopeShared:
		return "", "", ErrSharedKVDisabled
	default:
		return "", "", errors.New("scope must be personal or shared")
	}
}

func (s *Service) ListKV(ctx context.Context, appID, rawScope, prefix string) ([]storage.AppKVEntry, int64, int64, error) {
	scope, owner, err := resolveLocalKVScope(rawScope)
	if err != nil {
		return nil, 0, 0, err
	}
	if _, _, err := s.authorizeCapability(ctx, appID, appbundle.CapStoragePersonal); err != nil {
		return nil, 0, 0, err
	}
	items, total, err := s.store.ListAppKV(ctx, appID, scope, owner, prefix)
	return items, total, storage.DefaultMaxAppKVBytes, err
}

func (s *Service) GetKV(ctx context.Context, appID, rawScope, key string) (storage.AppKVEntry, error) {
	scope, owner, err := resolveLocalKVScope(rawScope)
	if err != nil {
		return storage.AppKVEntry{}, err
	}
	if _, _, err := s.authorizeCapability(ctx, appID, appbundle.CapStoragePersonal); err != nil {
		return storage.AppKVEntry{}, err
	}
	return s.store.GetAppKV(ctx, appID, scope, owner, key)
}

func (s *Service) PutKV(ctx context.Context, appID, rawScope, key, value string, expectedRev *int64) (storage.AppKVEntry, int64, error) {
	scope, owner, err := resolveLocalKVScope(rawScope)
	if err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	if _, _, err := s.authorizeCapability(ctx, appID, appbundle.CapStoragePersonal); err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	return s.store.PutAppKV(ctx, appID, scope, owner, key, value, storage.AppKVLocalOwnerID, expectedRev, storage.DefaultMaxAppKVBytes)
}

func (s *Service) DeleteKV(ctx context.Context, appID, rawScope, key string, expectedRev *int64) (int64, error) {
	scope, owner, err := resolveLocalKVScope(rawScope)
	if err != nil {
		return 0, err
	}
	if _, _, err := s.authorizeCapability(ctx, appID, appbundle.CapStoragePersonal); err != nil {
		return 0, err
	}
	return s.store.DeleteAppKV(ctx, appID, scope, owner, key, expectedRev)
}

type AICompleteRequest struct {
	Prompt string
	System string
	Model  string
}

type AICompleteResult struct {
	Text         string         `json:"text"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	BilledUserID string         `json:"billed_user_id"`
	Usage        map[string]any `json:"usage"`
}

func (s *Service) AIComplete(ctx context.Context, appID string, req AICompleteRequest) (AICompleteResult, error) {
	if _, _, err := s.authorizeCapability(ctx, appID, appbundle.CapAIComplete); err != nil {
		return AICompleteResult{}, err
	}
	if s.provider == nil {
		return AICompleteResult{}, ErrAIUnavailable
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" || utf8.RuneCountInString(prompt) > MaxAIPromptRunes {
		return AICompleteResult{}, errors.New("prompt is required and must be 64000 characters or fewer")
	}
	system := strings.TrimSpace(req.System)
	if utf8.RuneCountInString(system) > MaxAISystemRunes {
		return AICompleteResult{}, errors.New("system prompt must be 16000 characters or fewer")
	}
	msgs := make([]provider.Message, 0, 2)
	if system != "" {
		msgs = append(msgs, provider.SystemMessage(system))
	}
	msgs = append(msgs, provider.UserMessage(prompt))
	model := strings.TrimSpace(req.Model)
	resp, err := s.provider.Complete(ctx, provider.Request{
		Model:    model,
		Messages: msgs,
	})
	if err != nil {
		return AICompleteResult{}, err
	}
	return AICompleteResult{
		Text:         provider.MessageContent(resp.Message),
		Model:        model,
		StopReason:   "stop",
		BilledUserID: storage.AppKVLocalOwnerID,
		Usage: map[string]any{
			"input_tokens":        resp.Usage.InputTokens,
			"cached_input_tokens": resp.Usage.CachedInputTokens,
			"output_tokens":       resp.Usage.OutputTokens,
		},
	}, nil
}

func (s *Service) CallTool(ctx context.Context, appID, toolName string, arguments json.RawMessage) (any, error) {
	_, ver, err := s.authorizeCapability(ctx, appID, appbundle.CapToolsCall)
	if err != nil {
		return nil, err
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, errors.New("tool name is required")
	}
	if !slices.Contains(ver.AllowedTools, toolName) {
		return nil, ErrToolNotAllowed
	}
	if s.toolBridge == nil {
		return nil, errors.New("MCP tool bridge is not configured")
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	return s.toolBridge.CallAppBridgeTool(ctx, toolName, arguments)
}

func (s *Service) AssignToBoard(ctx context.Context, boardID string, appIDs []string) error {
	return s.store.AssignAppsToBoard(ctx, boardID, appIDs)
}

func (s *Service) Policy() storage.OrgAppPolicy {
	return storage.DefaultLocalAppPolicy()
}
