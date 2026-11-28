package server

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	mcpapi "github.com/wins/jaz/backend/internal/httpapi/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

const mcpOAuthCallbackPath = mcpconfig.OAuthCallbackPath

type mcpStore interface {
	mcpconfig.Store
}

type mcpProxyRuntime interface {
	Handler() http.Handler
}

type mcpOAuthCallbackRuntime interface {
	CompleteAuthorization(context.Context, mcpconfig.AuthorizationResponse) error
}

type mcpProxySessionStore interface {
	LoadSession(string) (storage.Session, error)
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	if s.MCP == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp runtime is not configured"))
		return
	}
	servers, err := s.MCP.ListMCPServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]mcpapi.ServerView, 0, len(servers))
	for _, server := range servers {
		out = append(out, s.mcpServerView(server))
	}
	redirectURL := mcpapi.AuthorizeOptions(r, s.ServerConfig.PublicBaseURL()).RedirectURL
	if runtime, ok := s.MCP.(interface{ OAuthRedirectURL(string) string }); ok {
		redirectURL = runtime.OAuthRedirectURL(redirectURL)
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out, "oauth_redirect_url": redirectURL})
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	store, ok := s.Store.(mcpStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp store is not configured"))
		return
	}
	input, err := mcpapi.DecodeServerInput(r, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	input.WorkspaceID = s.activeWorkspaceID()
	server, err := store.CreateMCPServer(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refreshMCP()
	writeJSON(w, http.StatusOK, s.mcpServerView(server))
}

func (s *Server) loadActiveMCPServer(store mcpStore, id string) (mcpconfig.Server, error) {
	current, err := store.LoadMCPServer(id)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if storage.NormalizeWorkspaceID(current.WorkspaceID) != s.activeWorkspaceID() {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return current, nil
}

func (s *Server) handleMCPServerAction(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	store, ok := s.Store.(mcpStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp store is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/mcp/servers/")
	id, action, hasAction := strings.Cut(rest, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mcp server id is required"))
		return
	}

	switch r.Method {
	case http.MethodPut:
		if hasAction {
			writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
			return
		}
		current, err := s.loadActiveMCPServer(store, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		input, err := mcpapi.DecodeServerInput(r, &current)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		server, err := store.UpdateMCPServer(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		s.refreshMCP()
		writeJSON(w, http.StatusOK, s.mcpServerView(server))
	case http.MethodDelete:
		if hasAction {
			writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
			return
		}
		if _, err := s.loadActiveMCPServer(store, id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err := store.DeleteMCPServer(id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		s.refreshMCP()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodPost:
		if !hasAction {
			writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
			return
		}
		s.handleMCPServerPostAction(w, r, store, id, action)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) handleMCPServerPostAction(w http.ResponseWriter, r *http.Request, store mcpStore, id, action string) {
	switch action {
	case "enable", "disable":
		if _, err := s.loadActiveMCPServer(store, id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		server, err := store.SetMCPServerEnabled(id, action == "enable")
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		s.refreshMCP()
		writeJSON(w, http.StatusOK, s.mcpServerView(server))
	case "test":
		if s.MCP == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp runtime is not configured"))
			return
		}
		server, err := mcpconfig.FindServer(s.MCP, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		ctx, cancel := serverActionContext()
		defer cancel()
		if r.Body != nil && r.ContentLength != 0 {
			if server.Source != "" {
				writeError(w, http.StatusBadRequest, fmt.Errorf("server configuration is managed by its %s source", server.Source))
				return
			}
			input, err := mcpapi.DecodeServerInput(r, &server)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			server = input.Server(server.ID, server.CreatedAt, server.UpdatedAt)
			writeJSON(w, http.StatusOK, s.MCP.Test(ctx, server))
			return
		}
		writeJSON(w, http.StatusOK, s.MCP.RefreshServer(ctx, id))
	case "authorize":
		if s.MCP == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp runtime is not configured"))
			return
		}
		server, err := mcpconfig.FindServer(s.MCP, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		// The browser sign-in flow can take a while; allow generous time, bounded
		// by the request context (cancelled if the client disconnects).
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		writeJSON(w, http.StatusOK, s.MCP.Authorize(ctx, server, mcpapi.AuthorizeOptions(r, s.ServerConfig.PublicBaseURL())))
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func (s *Server) handleMCPOAuthCallback(w http.ResponseWriter, r *http.Request) {
	runtime, ok := s.MCP.(mcpOAuthCallbackRuntime)
	if !ok || runtime == nil {
		writeMCPOAuthCallbackHTML(w, http.StatusServiceUnavailable, "Connection failed", "MCP runtime is not configured", false)
		return
	}
	q := r.URL.Query()
	failure := q.Get("error")
	if err := runtime.CompleteAuthorization(r.Context(), mcpconfig.AuthorizationResponse{State: q.Get("state"), Code: q.Get("code"), Issuer: q.Get("iss"), Error: failure}); err != nil {
		writeMCPOAuthCallbackHTML(w, http.StatusBadRequest, "Connection failed", err.Error(), false)
		return
	}
	if failure != "" {
		writeMCPOAuthCallbackHTML(w, http.StatusBadRequest, "Connection failed", "Authorization was rejected: "+failure, false)
		return
	}
	writeMCPOAuthCallbackHTML(w, http.StatusOK, "Connected", "You can close this tab and return to Jaz.", true)
}

func writeMCPOAuthCallbackHTML(w http.ResponseWriter, status int, title, message string, autoClose bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	script := ""
	if autoClose {
		script = `<script>setTimeout(function(){window.close()},1200)</script>`
	}
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Jaz</title></head><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;color:#333;background:#fff"><main style="width:min(640px,calc(100vw - 48px));text-align:center"><h2 style="margin:0 0 10px;font-size:22px;font-weight:600">` + html.EscapeString(title) + `</h2><p style="margin:0;color:#666;font-size:14px;line-height:1.5">` + html.EscapeString(message) + `</p></main>` + script + `</body></html>`))
}

func (s *Server) refreshMCP() {
	if s.MCP == nil {
		return
	}
	go func() {
		ctx, cancel := serverActionContext()
		defer cancel()
		s.MCP.Refresh(ctx)
	}()
}

func (s *Server) mcpProxyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.TrimSpace(r.Header.Get(mcpsession.HeaderName))
		if sessionID == "" {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("missing %s", mcpsession.HeaderName))
			return
		}
		if store, ok := s.Store.(mcpProxySessionStore); ok {
			if _, err := store.LoadSession(sessionID); err != nil {
				writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid %s", mcpsession.HeaderName))
				return
			}
		}
		runtime, ok := s.MCP.(mcpProxyRuntime)
		if !ok || runtime == nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("mcp runtime is not configured"))
			return
		}
		runtime.Handler().ServeHTTP(w, r)
	})
}

func (s *Server) mcpServerView(server mcpconfig.Server) mcpapi.ServerView {
	status := mcpconfig.ServerStatus{Status: "disabled"}
	if server.Enabled {
		status = mcpconfig.ServerStatus{Status: "unknown"}
	}
	if s.MCP != nil {
		if live := s.MCP.Status(server.ID); live.Status != "" {
			status = live
		}
	}
	return mcpapi.NewServerView(server, status)
}
