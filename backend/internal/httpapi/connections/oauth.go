package connections

import (
	"html"
	"net/http"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
)

const googleOAuthCallbackPath = "/v1/connections/oauth/google/callback"

type OAuthHandler struct {
	OAuth *connections.OAuthService
}

func NewOAuthHandler(oauth *connections.OAuthService) OAuthHandler {
	return OAuthHandler{OAuth: oauth}
}

func (h OAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	start, err := h.OAuth.Start(r.Context(), r.PathValue("id"), oauthCallbackURL(r))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, start)
}

func (h OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.OAuth.Callback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error")); err != nil {
		writeCallbackHTML(w, http.StatusBadRequest, err.Error())
		return
	}
	writeCallbackHTML(w, http.StatusOK, "Gmail is connected. You can close this tab and return to Jaz.")
}

func oauthCallbackURL(r *http.Request) string {
	return httpapi.RequestBaseURL(r) + googleOAuthCallbackPath
}

func writeCallbackHTML(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Jaz</title></head><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;color:#333"><div style="text-align:center"><h2 style="font-weight:600">Jaz</h2><p style="color:#666">` + html.EscapeString(message) + `</p><script>setTimeout(function(){window.close()},1200)</script></div></body></html>`))
}
