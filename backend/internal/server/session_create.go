package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
)

type createSessionRequest struct {
	Slug      string `json:"slug,omitempty"`
	Title     string `json:"title,omitempty"`
	Runtime   string `json:"runtime,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Directory string `json:"directory,omitempty"`
	Worktree  bool   `json:"worktree,omitempty"`

	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	switch strings.TrimSpace(req.Runtime) {
	case "", storage.RuntimeACP:
		s.createACPSession(w, req)
		return
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported session runtime %q; use acp", req.Runtime))
		return
	}
}

func (s *Server) createACPSession(w http.ResponseWriter, req createSessionRequest) {
	if s.ACP == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("acp manager is not configured"))
		return
	}
	directory := strings.TrimSpace(req.Directory)
	if req.Worktree && directory == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("worktree requires a directory pointing at a git repository"))
		return
	}
	if directory == "" {
		directory = "."
	}
	ctx, cancel := serverActionContext()
	defer cancel()
	session, err := s.ACP.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:        strings.TrimSpace(req.Agent),
		Slug:            req.Slug,
		Title:           req.Title,
		Directory:       directory,
		Worktree:        req.Worktree,
		ModelProvider:   strings.TrimSpace(req.ModelProvider),
		Model:           strings.TrimSpace(req.Model),
		ReasoningEffort: strings.TrimSpace(req.ReasoningEffort),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionview.Public(session))
}
