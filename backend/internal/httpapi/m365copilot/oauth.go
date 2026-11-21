package m365copilot

import (
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	core "github.com/wins/jaz/backend/internal/m365copilot"
)

type Handler struct {
	auth *core.AuthService
}

func NewHandler(auth *core.AuthService) *Handler {
	return &Handler{auth: auth}
}

func CallbackURL(r *http.Request, publicBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		base = httpapi.RequestBaseURL(r)
	}
	if base == "" {
		return ""
	}
	return base + core.OAuthCallbackPath
}

func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.auth.CompleteCallback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error"), q.Get("error_description")); err != nil {
		httpapi.WriteOAuthCallbackHTML(w, http.StatusBadRequest, "Connection failed", err.Error(), false)
		return
	}
	httpapi.WriteOAuthCallbackHTML(w, http.StatusOK, "Connected", "You can close this tab and return to Jaz.", true)
}
