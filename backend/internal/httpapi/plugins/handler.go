// Package plugins serves the unified /v1/plugins HTTP API.
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
	mcpapi "github.com/wins/jaz/backend/internal/httpapi/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/storage"
)

type MCPRuntime interface {
	Refresh(context.Context)
	RefreshServersAsync(...string)
}

type Handler struct {
	service         *plugins.Service
	sessions        plugins.SessionStore
	runtime         MCPRuntime
	callbackBaseURL string
}

func NewHandler(service *plugins.Service, sessions plugins.SessionStore, runtime MCPRuntime, callbackBaseURL string) *Handler {
	return &Handler{
		service:         service,
		sessions:        sessions,
		runtime:         runtime,
		callbackBaseURL: strings.TrimRight(strings.TrimSpace(callbackBaseURL), "/"),
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.List(r.Context(), mcpapi.AuthorizeOptions(r, h.callbackBaseURL).LoopbackAllowed)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	plugin, err := h.service.Get(r.Context(), r.PathValue("plugin"), mcpapi.AuthorizeOptions(r, h.callbackBaseURL).LoopbackAllowed)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, plugin)
}

func (h *Handler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plugin, err := h.service.SetEnabled(r.Context(), r.PathValue("plugin"), input.Enabled)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, plugin)
	h.refreshMCP()
}

func (h *Handler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plugin, err := h.service.SaveConfig(r.Context(), r.PathValue("plugin"), input.Values)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, plugin)
	h.refreshMCP()
}

func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Connect(r.Context(), r.PathValue("plugin"), connections.StartOptions{
		OAuthRedirectURL: h.callbackURL(r),
		MCP:              mcpapi.AuthorizeOptions(r, h.callbackBaseURL),
	})
	if err != nil {
		if errors.Is(err, connections.ErrQRProviderUnavailable) {
			httpapi.WriteError(w, http.StatusServiceUnavailable, err)
			return
		}
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	if result.MCPServersChanged {
		h.refreshMCP()
	}
	httpapi.WriteJSON(w, http.StatusOK, result.Start)
}

func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if h.service.OAuth() == nil {
		writeCallbackHTML(w, http.StatusServiceUnavailable, "Connection failed", "OAuth is not configured.", false)
		return
	}
	if err := h.service.OAuth().Callback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error")); err != nil {
		writeCallbackHTML(w, http.StatusBadRequest, "Connection failed", err.Error(), false)
		return
	}
	h.refreshMCP()
	writeCallbackHTML(w, http.StatusOK, "Connected", "You can close this tab and return to Jaz.", true)
}

func (h *Handler) DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.DisconnectAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, connections.ErrConnectionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if result.MCPServersChanged {
		h.refreshMCP()
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) QRStatus(w http.ResponseWriter, r *http.Request) {
	if h.service.QR() == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, errors.New("QR login unavailable"))
		return
	}
	status, err := h.service.QR().Status(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, connections.ErrQRSessionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) QRPassword(w http.ResponseWriter, r *http.Request) {
	if h.service.QR() == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, errors.New("QR login unavailable"))
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid QR password request"))
		return
	}
	if input.Password == "" {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("QR password is required"))
		return
	}
	status, err := h.service.QR().SubmitPassword(r.Context(), r.PathValue("id"), input.Password)
	if err != nil {
		if errors.Is(err, connections.ErrQRSessionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, connections.ErrQRPasswordNotRequired) {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) CloseQR(w http.ResponseWriter, r *http.Request) {
	if h.service.QR() == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, errors.New("QR login unavailable"))
		return
	}
	if err := h.service.QR().Close(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, connections.ErrQRSessionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	page, err := h.service.Store().PageWithContext(r.Context(), r.PathValue("plugin"), h.sessions)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) Install(w http.ResponseWriter, r *http.Request) {
	var input plugins.RepositoryInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.Install(r.Context(), input)
	h.writeInstalled(w, result, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Update(r.Context(), r.PathValue("plugin"))
	h.writeInstalled(w, result, err)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Remove(r.Context(), r.PathValue("plugin")); err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	if h.runtime != nil {
		h.runtime.RefreshServersAsync()
	}
}

func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.service.ListSources(r.Context())
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

func (h *Handler) AddSource(w http.ResponseWriter, r *http.Request) {
	var input plugins.RepositoryInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	source, err := h.service.AddSource(r.Context(), input)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, source)
}

func (h *Handler) RemoveSource(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RemoveSource(r.Context(), r.PathValue("id")); err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RefreshSources(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RefreshSources(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, err)
		return
	}
	resp, err := h.service.List(r.Context(), mcpapi.AuthorizeOptions(r, h.callbackBaseURL).LoopbackAllowed)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeInstalled(w http.ResponseWriter, result plugins.InstallResult, err error) {
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
	var ids []string
	for _, plugin := range result.Plugins {
		for _, srv := range plugin.MCPServers {
			ids = append(ids, plugins.MCPServerID(plugin.ID, srv.Name))
		}
	}
	if h.runtime != nil {
		h.runtime.RefreshServersAsync(ids...)
	}
}

func (h *Handler) refreshMCP() {
	if h.runtime != nil {
		go h.runtime.Refresh(context.Background())
	}
}

func (h *Handler) callbackURL(r *http.Request) string {
	base := h.callbackBaseURL
	if base == "" {
		base = httpapi.RequestBaseURL(r)
	}
	return base + plugins.OAuthCallbackPath
}

func writeCallbackHTML(w http.ResponseWriter, status int, title, message string, autoClose bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	script := ""
	if autoClose {
		script = `<script>setTimeout(function(){window.close()},1200)</script>`
	}
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Jaz</title></head><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;color:#333;background:#fff"><main style="width:min(640px,calc(100vw - 48px));text-align:center"><h2 style="margin:0 0 10px;font-size:22px;font-weight:600">` + html.EscapeString(title) + `</h2><p style="margin:0;color:#666;font-size:14px;line-height:1.5">` + html.EscapeString(message) + `</p></main>` + script + `</body></html>`))
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, plugins.ErrNotFound), errors.Is(err, storage.ErrSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, plugins.ErrConflict), errors.Is(err, storage.ErrPluginStateConflict):
		return http.StatusConflict
	case errors.Is(err, plugins.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
