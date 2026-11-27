package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wins/jaz/backend/internal/appbundle"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

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
