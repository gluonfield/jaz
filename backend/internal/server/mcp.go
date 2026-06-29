package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type mcpStore interface {
	mcpconfig.Store
}

type mcpProxyRuntime interface {
	Handler() http.Handler
}

type mcpProxySessionStore interface {
	LoadSession(string) (storage.Session, error)
}

type mcpServerInput struct {
	Name              string                `json:"name"`
	URL               string                `json:"url"`
	Enabled           *bool                 `json:"enabled,omitempty"`
	BearerTokenEnvVar string                `json:"bearer_token_env_var,omitempty"`
	Headers           []mcpconfig.Header    `json:"headers,omitempty"`
	EnvHeaders        []mcpconfig.EnvHeader `json:"env_headers,omitempty"`
	OAuth             mcpconfig.OAuthConfig `json:"oauth,omitempty"`
}

type mcpServerView struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Transport         string                 `json:"transport"`
	URL               string                 `json:"url"`
	Enabled           bool                   `json:"enabled"`
	BearerTokenEnvVar string                 `json:"bearer_token_env_var,omitempty"`
	Headers           []mcpconfig.Header     `json:"headers,omitempty"`
	EnvHeaders        []mcpconfig.EnvHeader  `json:"env_headers,omitempty"`
	OAuth             mcpconfig.OAuthConfig  `json:"oauth,omitempty"`
	Status            string                 `json:"status"`
	ToolCount         int                    `json:"tool_count"`
	Tools             []mcpconfig.ServerTool `json:"tools,omitempty"`
	Error             string                 `json:"error,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	store, ok := s.Store.(mcpStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp store is not configured"))
		return
	}
	servers, err := store.ListMCPServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]mcpServerView, 0, len(servers))
	for _, server := range servers {
		out = append(out, s.mcpServerView(server))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	store, ok := s.Store.(mcpStore)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp store is not configured"))
		return
	}
	input, err := decodeMCPServerInput(r, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	server, err := store.CreateMCPServer(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refreshMCP()
	writeJSON(w, http.StatusOK, s.mcpServerView(server))
}

func (s *Server) handleMCPServerAction(w http.ResponseWriter, r *http.Request) {
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
		current, err := store.LoadMCPServer(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		input, err := decodeMCPServerInput(r, &current)
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
		server, err := store.SetMCPServerEnabled(id, action == "enable")
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		s.refreshMCP()
		writeJSON(w, http.StatusOK, s.mcpServerView(server))
	case "test":
		server, err := store.LoadMCPServer(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if r.Body != nil && r.ContentLength != 0 {
			if input, err := decodeMCPServerInput(r, &server); err == nil {
				server.Name = input.Name
				server.URL = input.URL
				server.Enabled = input.Enabled
				server.BearerTokenEnvVar = input.BearerTokenEnvVar
				server.Headers = input.Headers
				server.EnvHeaders = input.EnvHeaders
				server.OAuth = input.OAuth
			}
		}
		if s.MCP == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp runtime is not configured"))
			return
		}
		ctx, cancel := serverActionContext()
		defer cancel()
		writeJSON(w, http.StatusOK, s.MCP.Test(ctx, server))
	case "authorize":
		server, err := store.LoadMCPServer(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if s.MCP == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("mcp runtime is not configured"))
			return
		}
		// The browser sign-in flow can take a while; allow generous time, bounded
		// by the request context (cancelled if the client disconnects).
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		writeJSON(w, http.StatusOK, s.MCP.Authorize(ctx, server))
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func decodeMCPServerInput(r *http.Request, current *mcpconfig.Server) (mcpconfig.ServerInput, error) {
	var req mcpServerInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return mcpconfig.ServerInput{}, err
	}
	enabled := true
	if current != nil {
		enabled = current.Enabled
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return mcpconfig.ValidateInput(mcpconfig.ServerInput{
		Name:              req.Name,
		URL:               req.URL,
		Enabled:           enabled,
		BearerTokenEnvVar: req.BearerTokenEnvVar,
		Headers:           req.Headers,
		EnvHeaders:        req.EnvHeaders,
		OAuth:             req.OAuth,
	})
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

func (s *Server) mcpServerView(server mcpconfig.Server) mcpServerView {
	status := mcpconfig.ServerStatus{Status: "disabled"}
	if server.Enabled {
		status = mcpconfig.ServerStatus{Status: "unknown"}
	}
	if s.MCP != nil {
		if live := s.MCP.Status(server.ID); live.Status != "" {
			status = live
		}
	}
	return mcpServerView{
		ID:                server.ID,
		Name:              server.Name,
		Transport:         server.Transport,
		URL:               server.URL,
		Enabled:           server.Enabled,
		BearerTokenEnvVar: server.BearerTokenEnvVar,
		Headers:           server.Headers,
		EnvHeaders:        server.EnvHeaders,
		OAuth:             server.OAuth,
		Status:            status.Status,
		ToolCount:         status.ToolCount,
		Tools:             status.Tools,
		Error:             status.Error,
		CreatedAt:         server.CreatedAt,
		UpdatedAt:         server.UpdatedAt,
	}
}
