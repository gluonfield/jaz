package apps

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	appscore "github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

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
