package apps

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	appscore "github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/httpapi"
)

var appContentSandboxCSP = strings.Join([]string{
	"sandbox allow-scripts allow-forms",
	"default-src 'none'",
	"script-src 'unsafe-inline' 'wasm-unsafe-eval' https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com",
	"style-src 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com",
	"font-src data: https://fonts.gstatic.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com",
	"img-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://fonts.gstatic.com https://fonts.googleapis.com",
	"connect-src https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com",
	"media-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com",
	"worker-src 'none'",
	"frame-src 'none'",
	"object-src 'none'",
	"base-uri 'none'",
	"form-action 'none'",
}, "; ")

func IsAppContentRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/app-content/")
}

func setAppContentSecurityHeaders(w http.ResponseWriter) {
	w.Header().Del("X-Frame-Options")
	w.Header().Del("Set-Cookie")
	w.Header().Del("Access-Control-Allow-Credentials")
	w.Header().Set("Content-Security-Policy", appContentSandboxCSP)
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
}

// ServeContent serves /app-content/{id} and /app-content/{id}/{path...} as a
// public, cookieless, ticket-authenticated sandboxed endpoint (or proxies to
// the active Cloud workspace when one is selected).
func (h *Handler) ServeContent(w http.ResponseWriter, r *http.Request) {
	if h.cloud != nil && h.cloud.ProxyActiveWorkspaceAppContent(w, r) {
		return
	}
	setAppContentSecurityHeaders(w)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpapi.WriteError(w, http.StatusMethodNotAllowed, errors.New("only GET and HEAD are allowed on app content endpoints"))
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Service-Worker")), "script") {
		httpapi.WriteError(w, http.StatusForbidden, errors.New("service worker registration is forbidden on app content origin"))
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/app-content/")
	appID, relpath, _ := strings.Cut(rest, "/")
	appID = strings.TrimSpace(appID)
	if !appscore.IsValidAppID(appID) {
		httpapi.WriteError(w, http.StatusNotFound, errors.New("app not found"))
		return
	}

	ticket := strings.TrimSpace(r.Header.Get("X-Jaz-App-Ticket"))
	if ticket == "" {
		ticket = strings.TrimSpace(r.URL.Query().Get("ticket"))
	}
	if ticket == "" {
		httpapi.WriteError(w, http.StatusUnauthorized, errors.New("app access ticket required"))
		return
	}
	claims, err := h.service.ParseTicket(ticket)
	if err != nil || claims.AppID != appID {
		httpapi.WriteError(w, http.StatusUnauthorized, errors.New("invalid or out-of-scope app access ticket"))
		return
	}

	fileRead, err := h.service.ReadBundleFile(r.Context(), claims, relpath)
	if err != nil {
		writeAppError(w, err)
		return
	}

	w.Header().Set("Content-Type", fileRead.ContentType)
	w.Header().Set("X-Jaz-App-Bundle-SHA256", fileRead.BundleSHA256)
	w.Header().Set("Content-Length", strconv.Itoa(len(fileRead.Data)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(fileRead.Data)
}
