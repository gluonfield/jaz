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
	appIDLength      = 26
	appIDAlphabet    = "abcdefghijklmnopqrstuvwxyz0123456789"
	appDraftTTL      = time.Hour
	MaxKVValueBytes  = 2 << 20
	MaxAIPromptRunes = 64000
	MaxAISystemRunes = 16000
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
		if _, err := s.store.GetApp(ctx, targetAppID); err != nil {
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
