package connections

import (
	"errors"
	"html"
	"net/http"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
)

const googleOAuthCallbackPath = "/v1/connections/oauth/google/callback"

type ConnectHandler struct {
	Connect *connections.ConnectService
	OAuth   *connections.OAuthService
	QR      *connections.QRService
}

func NewConnectHandler(connect *connections.ConnectService, oauth *connections.OAuthService, qr *connections.QRService) ConnectHandler {
	return ConnectHandler{Connect: connect, OAuth: oauth, QR: qr}
}

func (h ConnectHandler) Start(w http.ResponseWriter, r *http.Request) {
	start, err := h.Connect.Start(r.Context(), r.PathValue("id"), oauthCallbackURL(r))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, start)
}

func (h ConnectHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.OAuth.Callback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error")); err != nil {
		writeCallbackHTML(w, http.StatusBadRequest, "Gmail connection failed", err.Error(), false)
		return
	}
	writeCallbackHTML(w, http.StatusOK, "Gmail connected", "You can close this tab and return to Jaz.", true)
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

func oauthCallbackURL(r *http.Request) string {
	return httpapi.RequestBaseURL(r) + googleOAuthCallbackPath
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
