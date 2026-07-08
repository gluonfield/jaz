package connections

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
)

// OAuthCallbackPath is the shared redirect target for every OAuth provider; the
// provider is recovered from the OAuth state, not the URL.
const OAuthCallbackPath = "/v1/connections/oauth/callback"

type ConnectHandler struct {
	Connect *connections.ConnectService
	OAuth   *connections.OAuthService
	QR      *connections.QRService
	MCP     MCPRefresher
	// CallbackBaseURL is the trusted public origin for OAuth redirect URIs. When
	// empty the request host is used. Providers like Slack require an HTTPS
	// callback, configured here rather than derived from request headers.
	CallbackBaseURL string
}

type qrPasswordRequest struct {
	Password string `json:"password"`
}

func NewConnectHandler(connect *connections.ConnectService, oauth *connections.OAuthService, qr *connections.QRService, mcp MCPRefresher, callbackBaseURL string) ConnectHandler {
	return ConnectHandler{Connect: connect, OAuth: oauth, QR: qr, MCP: mcp, CallbackBaseURL: strings.TrimRight(strings.TrimSpace(callbackBaseURL), "/")}
}

func (h ConnectHandler) Start(w http.ResponseWriter, r *http.Request) {
	result, err := h.Connect.Start(r.Context(), r.PathValue("id"), h.callbackURL(r))
	if err != nil {
		if errors.Is(err, connections.ErrQRProviderUnavailable) {
			httpapi.WriteError(w, http.StatusServiceUnavailable, err)
			return
		}
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if result.MCPServersChanged {
		refreshMCP(h.MCP)
	}
	httpapi.WriteJSON(w, http.StatusOK, result.Start)
}

func (h ConnectHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.OAuth.Callback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error")); err != nil {
		writeCallbackHTML(w, http.StatusBadRequest, "Connection failed", err.Error(), false)
		return
	}
	writeCallbackHTML(w, http.StatusOK, "Connected", "You can close this tab and return to Jaz.", true)
}

func (h ConnectHandler) QRStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.QR.Status(r.Context(), r.PathValue("id"))
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

func (h ConnectHandler) QRPassword(w http.ResponseWriter, r *http.Request) {
	var input qrPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid QR password request"))
		return
	}
	if input.Password == "" {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("QR password is required"))
		return
	}
	status, err := h.QR.SubmitPassword(r.Context(), r.PathValue("id"), input.Password)
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

func (h ConnectHandler) CloseQR(w http.ResponseWriter, r *http.Request) {
	if err := h.QR.Close(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, connections.ErrQRSessionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h ConnectHandler) callbackURL(r *http.Request) string {
	base := h.CallbackBaseURL
	if base == "" {
		base = httpapi.RequestBaseURL(r)
	}
	return base + OAuthCallbackPath
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
