package apps

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
		errors.Is(err, storage.ErrAppApprovalRequired),
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
	if kind != "" && kind != storage.AppKindArtifact && kind != storage.AppKindDocument && kind != storage.AppKindBoard {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("kind must be artifact, document or board"))
		return
	}
	items, err := h.service.ListApps(r.Context(), storage.AppListFilter{
		Scope:          scope,
		SpaceID:        strings.TrimSpace(q.Get("space_id")),
		SourceThreadID: strings.TrimSpace(q.Get("source_thread_id")),
		Kind:           kind,
		Query:          strings.TrimSpace(q.Get("q")),
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
		"draft":         draft,
		"draft_id":      draft.ID,
		"bundle_sha256": draft.BundleSHA256,
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
		createDraftBody
		DraftID      string `json:"draft_id"`
		BundleSHA256 string `json:"bundle_sha256"`
		Visibility   string `json:"visibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDraftBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	if requestedID == "new" && body.Kind == storage.AppKindBoard {
		board, err := h.service.CreateBoard(r.Context(), body.Title)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"app": board})
		return
	}
	var (
		app   storage.App
		ver   storage.AppVersion
		isNew bool
		err   error
	)
	if strings.TrimSpace(body.DraftID) == "" {
		bundle, packErr := packDraftBody(body.createDraftBody)
		if packErr != nil {
			writeBundleError(w, packErr)
			return
		}
		targetID := requestedID
		if targetID == "new" {
			targetID = ""
		}
		app, ver, isNew, err = h.service.PublishBundle(r.Context(), bundle, targetID, strings.TrimSpace(body.SourceThreadID))
	} else {
		app, ver, isNew, err = h.service.PublishFromDraft(r.Context(), appscore.PublishInput{
			RequestedID:  requestedID,
			DraftID:      body.DraftID,
			BundleSHA256: body.BundleSHA256,
			Title:        body.Title,
			Description:  body.Description,
			Visibility:   body.Visibility,
		})
	}
	if err != nil {
		if errors.Is(err, storage.ErrAppNotFound) || errors.Is(err, storage.ErrAppForbidden) ||
			errors.Is(err, storage.ErrAppApprovalRequired) ||
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
