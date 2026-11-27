package apps

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wins/jaz/backend/internal/appbundle"
	appscore "github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

const maxDraftBodyBytes int64 = 32 << 20

type CloudProxy interface {
	ProxyActiveWorkspace(w http.ResponseWriter, r *http.Request) bool
	ProxyActiveWorkspaceAppContent(w http.ResponseWriter, r *http.Request) bool
}

type Handler struct {
	service *appscore.Service
	cloud   CloudProxy
	baseURL string
}

func NewHandler(service *appscore.Service, baseURL string) *Handler {
	return &Handler{
		service: service,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

func (h *Handler) WithCloud(cloud CloudProxy) *Handler {
	h.cloud = cloud
	return h
}

func (h *Handler) proxyCloud(w http.ResponseWriter, r *http.Request) bool {
	return h.cloud != nil && h.cloud.ProxyActiveWorkspace(w, r)
}

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrAppNotFound):
		httpapi.WriteError(w, http.StatusNotFound, err)
	case errors.Is(err, storage.ErrAppForbidden),
		errors.Is(err, appscore.ErrSharedKVDisabled),
		errors.Is(err, appscore.ErrCapabilityDenied),
		errors.Is(err, appscore.ErrToolNotAllowed):
		httpapi.WriteError(w, http.StatusForbidden, err)
	case errors.Is(err, storage.ErrAppConflict),
		errors.Is(err, storage.ErrAppDraftConsumed),
		errors.Is(err, storage.ErrAppDraftMismatch):
		httpapi.WriteError(w, http.StatusConflict, err)
	case errors.Is(err, storage.ErrAppKVQuotaExceeded),
		errors.Is(err, appbundle.ErrBundleTooLarge),
		errors.Is(err, appbundle.ErrFileTooLarge):
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
	case errors.Is(err, storage.ErrAppKVConflict):
		httpapi.WriteError(w, http.StatusPreconditionFailed, err)
	case errors.Is(err, appscore.ErrAIUnavailable):
		httpapi.WriteError(w, http.StatusServiceUnavailable, err)
	default:
		httpapi.WriteError(w, http.StatusInternalServerError, err)
	}
}

func writeBundleError(w http.ResponseWriter, err error) {
	if errors.Is(err, appbundle.ErrBundleTooLarge) || errors.Is(err, appbundle.ErrFileTooLarge) {
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	httpapi.WriteError(w, http.StatusBadRequest, err)
}

func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.service.Policy())
}

func (h *Handler) PutPolicy(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	httpapi.WriteError(w, http.StatusForbidden, errors.New("org app policies require a Jaz Cloud workspace"))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	q := r.URL.Query()
	scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
	if scope != "" && scope != "all" && scope != "mine" && scope != "space" && scope != "team" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("scope must be all, mine, space, or team"))
		return
	}
	kind := strings.ToLower(strings.TrimSpace(q.Get("kind")))
	if kind != "" && kind != storage.AppKindArtifact && kind != storage.AppKindDocument {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("kind must be artifact or document"))
		return
	}
	items, err := h.service.ListApps(r.Context(), storage.AppListFilter{
		Scope:   scope,
		SpaceID: strings.TrimSpace(q.Get("space_id")),
		Kind:    kind,
		Query:   strings.TrimSpace(q.Get("q")),
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	if items == nil {
		items = []storage.App{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"apps": items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	app, currentVer, err := h.service.GetApp(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"app":             app,
		"current_version": currentVer,
	})
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	versions, err := h.service.ListVersions(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeAppError(w, err)
		return
	}
	if versions == nil {
		versions = []storage.AppVersion{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

type draftFileInput struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type createDraftBody struct {
	TargetAppID    string              `json:"target_app_id"`
	SourceThreadID string              `json:"source_thread_id"`
	Manifest       *appbundle.Manifest `json:"manifest"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	Kind           string              `json:"kind"`
	Entry          string              `json:"entry"`
	Capabilities   []string            `json:"capabilities"`
	AllowedTools   []string            `json:"allowed_tools"`
	Icon           *appbundle.IconSpec `json:"icon"`
	HTML           string              `json:"html"`
	Markdown       string              `json:"markdown"`
	Content        string              `json:"content"`
	Files          []draftFileInput    `json:"files"`
}

func (h *Handler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDraftBodyBytes)
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	var (
		bundle         appbundle.Bundle
		targetAppID    string
		sourceThreadID string
		err            error
	)
	if strings.HasPrefix(ct, "application/zip") || strings.HasPrefix(ct, "application/octet-stream") {
		rawZip, rerr := io.ReadAll(r.Body)
		if rerr != nil {
			var maxErr *http.MaxBytesError
			if errors.As(rerr, &maxErr) {
				httpapi.WriteError(w, http.StatusRequestEntityTooLarge, errors.New("bundle upload exceeds limit"))
				return
			}
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("failed to read zip body"))
			return
		}
		targetAppID = strings.TrimSpace(r.URL.Query().Get("target_app_id"))
		sourceThreadID = strings.TrimSpace(r.URL.Query().Get("source_thread_id"))
		bundle, err = appbundle.PackZipBytes(rawZip)
		if err != nil {
			writeBundleError(w, err)
			return
		}
	} else {
		var body createDraftBody
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			var maxErr *http.MaxBytesError
			if errors.As(derr, &maxErr) {
				httpapi.WriteError(w, http.StatusRequestEntityTooLarge, errors.New("bundle upload exceeds limit"))
				return
			}
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
			return
		}
		targetAppID = strings.TrimSpace(body.TargetAppID)
		sourceThreadID = strings.TrimSpace(body.SourceThreadID)
		bundle, err = packDraftBody(body)
		if err != nil {
			writeBundleError(w, err)
			return
		}
	}

	draft, err := h.service.StageDraft(r.Context(), bundle, targetAppID, sourceThreadID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{
		"draft":                      draft,
		"draft_id":                   draft.ID,
		"bundle_sha256":              draft.BundleSHA256,
		"requires_user_confirmation": true,
	})
}

func packDraftBody(body createDraftBody) (appbundle.Bundle, error) {
	var m appbundle.Manifest
	if body.Manifest != nil {
		m = *body.Manifest
		if m.Format == "" {
			m.Format = appbundle.Format
		}
	} else {
		kind, entry := appscore.DefaultKindAndEntry(
			body.Kind,
			body.Entry,
			strings.TrimSpace(body.HTML) != "",
			strings.TrimSpace(body.Markdown) != "",
		)
		m = appbundle.Manifest{
			Format:       appbundle.Format,
			Title:        body.Title,
			Description:  body.Description,
			Kind:         kind,
			Entry:        entry,
			Capabilities: body.Capabilities,
			AllowedTools: body.AllowedTools,
			Icon:         body.Icon,
		}
	}
	files := make(map[string][]byte, len(body.Files)+1)
	for _, f := range body.Files {
		switch strings.ToLower(strings.TrimSpace(f.Encoding)) {
		case "", "utf8", "utf-8", "text":
			files[f.Path] = []byte(f.Content)
		case "base64":
			dec, err := base64.StdEncoding.DecodeString(f.Content)
			if err != nil {
				return appbundle.Bundle{}, fmt.Errorf("invalid base64 content for %q", f.Path)
			}
			files[f.Path] = dec
		default:
			return appbundle.Bundle{}, fmt.Errorf("unsupported file encoding %q for %q", f.Encoding, f.Path)
		}
	}
	if len(files) == 0 {
		inline := firstNonEmpty(body.HTML, body.Markdown, body.Content)
		if strings.TrimSpace(inline) != "" && m.Entry != "" {
			files[m.Entry] = []byte(inline)
		}
	}
	return appbundle.PackFiles(m, files)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	h.publish(w, r, "new")
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	h.publish(w, r, strings.TrimSpace(r.PathValue("id")))
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request, requestedID string) {
	if h.proxyCloud(w, r) {
		return
	}
	var body struct {
		DraftID      string `json:"draft_id"`
		BundleSHA256 string `json:"bundle_sha256"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Visibility   string `json:"visibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	app, ver, isNew, err := h.service.PublishFromDraft(r.Context(), appscore.PublishInput{
		RequestedID:  requestedID,
		DraftID:      body.DraftID,
		BundleSHA256: body.BundleSHA256,
		Title:        body.Title,
		Description:  body.Description,
		Visibility:   body.Visibility,
	})
	if err != nil {
		if errors.Is(err, storage.ErrAppNotFound) || errors.Is(err, storage.ErrAppForbidden) ||
			errors.Is(err, storage.ErrAppDraftConsumed) || errors.Is(err, storage.ErrAppDraftMismatch) {
			writeAppError(w, err)
			return
		}
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	httpapi.WriteJSON(w, status, map[string]any{
		"app":     app,
		"version": ver,
	})
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	appID := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Title       *string             `json:"title"`
		Description *string             `json:"description"`
		Icon        *appbundle.IconSpec `json:"icon"`
		Visibility  *string             `json:"visibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	var patch storage.AppPatch
	if body.Title != nil {
		trimmed := strings.TrimSpace(*body.Title)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > appbundle.MaxTitleRunes {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("title must be between 1 and 200 characters"))
			return
		}
		patch.Title = &trimmed
	}
	if body.Description != nil {
		trimmed := strings.TrimSpace(*body.Description)
		if utf8.RuneCountInString(trimmed) > appbundle.MaxDescRunes {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("description exceeds 2000 characters"))
			return
		}
		patch.Description = &trimmed
	}
	if body.Icon != nil {
		if err := appbundle.ValidateIcon(body.Icon); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		rawIcon, _ := json.Marshal(body.Icon)
		iconRaw := json.RawMessage(rawIcon)
		patch.IconJSON = &iconRaw
	}
	if body.Visibility != nil {
		vis, err := appscore.NormalizeVisibility(*body.Visibility)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		patch.Visibility = &vis
	}
	updated, err := h.service.UpdateApp(r.Context(), appID, patch)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil || body.Version <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("positive target version is required"))
		return
	}
	app, ver, err := h.service.Rollback(r.Context(), strings.TrimSpace(r.PathValue("id")), body.Version)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"app":     app,
		"version": ver,
	})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	appID := strings.TrimSpace(r.PathValue("id"))
	if err := h.service.DeleteApp(r.Context(), appID); err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": appID})
}

func (h *Handler) Remix(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
			return
		}
	}
	res, err := h.service.RemixApp(r.Context(), strings.TrimSpace(r.PathValue("id")), body.Title)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, res)
}

func (h *Handler) IssueTicket(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	appID := strings.TrimSpace(r.PathValue("id"))
	var version int
	if raw := strings.TrimSpace(r.URL.Query().Get("version")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("version must be a positive integer"))
			return
		}
		version = parsed
	} else if r.Body != nil && r.ContentLength != 0 {
		var body struct {
			Version int `json:"version"`
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil && body.Version > 0 {
			version = body.Version
		}
	}
	base := h.baseURL
	if base == "" && r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	resp, err := h.service.IssueTicket(r.Context(), appID, version, base)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func parseIfMatchRevision(r *http.Request, bodyExpected *int64) (*int64, error) {
	if raw := strings.TrimSpace(r.Header.Get("If-Match")); raw != "" {
		if raw == "*" {
			return nil, nil
		}
		cleaned := strings.TrimPrefix(strings.Trim(strings.TrimPrefix(raw, "W/"), `"`), "rev-")
		rev, err := strconv.ParseInt(cleaned, 10, 64)
		if err != nil || rev < 0 {
			return nil, errors.New("invalid If-Match revision header")
		}
		return &rev, nil
	}
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == "*" {
		var zero int64
		return &zero, nil
	}
	if bodyExpected != nil {
		if *bodyExpected < 0 {
			return nil, errors.New("expected_revision must be non-negative")
		}
		v := *bodyExpected
		return &v, nil
	}
	return nil, nil
}

func isValidKVKey(key string) bool {
	if key == "" || len(key) > 256 || strings.TrimSpace(key) != key {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] == 0x7f {
			return false
		}
	}
	return true
}

func (h *Handler) ListKV(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	items, total, quota, err := h.service.ListKV(
		r.Context(),
		strings.TrimSpace(r.PathValue("id")),
		r.URL.Query().Get("scope"),
		r.URL.Query().Get("prefix"),
	)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if items == nil {
		items = []storage.AppKVEntry{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":       storage.AppKVScopePersonal,
		"items":       items,
		"total_bytes": total,
		"quota_bytes": quota,
	})
}

func (h *Handler) GetKV(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	key := r.PathValue("key")
	if !isValidKVKey(key) {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid kv key"))
		return
	}
	entry, err := h.service.GetKV(r.Context(), strings.TrimSpace(r.PathValue("id")), r.URL.Query().Get("scope"), key)
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, entry.Revision))
	httpapi.WriteJSON(w, http.StatusOK, entry)
}

func (h *Handler) PutKV(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	key := r.PathValue("key")
	if !isValidKVKey(key) {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid kv key"))
		return
	}
	var body struct {
		Scope            string `json:"scope"`
		Value            string `json:"value"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, appscore.MaxKVValueBytes+4096)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	if !utf8.ValidString(body.Value) {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("kv value must be valid UTF-8 text"))
		return
	}
	if len(body.Value) > appscore.MaxKVValueBytes {
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, errors.New("kv value exceeds 2 MiB per-key limit"))
		return
	}
	expectedRev, err := parseIfMatchRevision(r, body.ExpectedRevision)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	entry, curRev, err := h.service.PutKV(
		r.Context(),
		strings.TrimSpace(r.PathValue("id")),
		firstNonEmpty(body.Scope, r.URL.Query().Get("scope")),
		key,
		body.Value,
		expectedRev,
	)
	if err != nil {
		if errors.Is(err, storage.ErrAppKVConflict) {
			httpapi.WriteJSON(w, http.StatusPreconditionFailed, map[string]any{
				"error":            err.Error(),
				"current_revision": curRev,
			})
			return
		}
		writeAppError(w, err)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, entry.Revision))
	httpapi.WriteJSON(w, http.StatusOK, entry)
}

func (h *Handler) DeleteKV(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	key := r.PathValue("key")
	if !isValidKVKey(key) {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid kv key"))
		return
	}
	expectedRev, err := parseIfMatchRevision(r, nil)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	curRev, err := h.service.DeleteKV(r.Context(), strings.TrimSpace(r.PathValue("id")), r.URL.Query().Get("scope"), key, expectedRev)
	if err != nil {
		if errors.Is(err, storage.ErrAppKVConflict) {
			httpapi.WriteJSON(w, http.StatusPreconditionFailed, map[string]any{
				"error":            err.Error(),
				"current_revision": curRev,
			})
			return
		}
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "key": key, "scope": storage.AppKVScopePersonal})
}

func (h *Handler) AIComplete(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
		System string `json:"system"`
		Model  string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	res, err := h.service.AIComplete(r.Context(), strings.TrimSpace(r.PathValue("id")), appscore.AICompleteRequest{
		Prompt: body.Prompt,
		System: body.System,
		Model:  body.Model,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) CallTool(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body struct {
		Tool      string          `json:"tool"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	toolName := strings.TrimSpace(firstNonEmpty(body.Tool, body.Name))
	if toolName == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("tool name is required"))
		return
	}
	res, err := h.service.CallTool(r.Context(), strings.TrimSpace(r.PathValue("id")), toolName, body.Arguments)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"tool":   toolName,
		"result": res,
	})
}

func (h *Handler) AssignToBoard(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	boardID := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		AppIDs []string `json:"app_ids"`
		AppID  string   `json:"app_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	ids := body.AppIDs
	if len(ids) == 0 && strings.TrimSpace(body.AppID) != "" {
		ids = []string{strings.TrimSpace(body.AppID)}
	}
	if len(ids) == 0 || len(ids) > 50 {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("app_ids must contain between 1 and 50 items"))
		return
	}
	if err := h.service.AssignToBoard(r.Context(), boardID, ids); err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
