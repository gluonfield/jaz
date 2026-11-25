package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

type createSessionRequest struct {
	Slug      string `json:"slug,omitempty"`
	Title     string `json:"title,omitempty"`
	Runtime   string `json:"runtime,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Directory string `json:"directory,omitempty"`
	Worktree  bool   `json:"worktree,omitempty"`
	// ArtifactSurface is fixed at creation: "chat" (default) or "canvas".
	ArtifactSurface string `json:"artifact_surface,omitempty"`

	Backend         string `json:"backend,omitempty"`
	ModelProvider   string `json:"model_provider,omitempty"`
	ModelProfileID  string `json:"model_profile_id,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	SpaceID         string `json:"space_id,omitempty"`
	MachineID       string `json:"machine_id,omitempty"`
	ExecutorTarget  string `json:"executor_target,omitempty"`
	GitRepo         string `json:"git_repo,omitempty"`
	GitBaseBranch   string `json:"git_base_branch,omitempty"`
	GitBranch       string `json:"git_branch,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if strings.EqualFold(strings.TrimSpace(req.Backend), "cloud") || strings.TrimSpace(req.ModelProfileID) != "" {
		s.createCloudSession(w, r, req)
		return
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

func (s *Server) createCloudSession(w http.ResponseWriter, r *http.Request, req createSessionRequest) {
	if s.Cloud == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("Jaz Cloud is not configured"))
		return
	}
	target := strings.TrimSpace(req.ExecutorTarget)
	if target == "" {
		target = strings.TrimSpace(req.MachineID)
	}
	session, err := s.Cloud.CreateThread(r.Context(), cloudsync.CreateThreadRequest{
		Title:           strings.TrimSpace(req.Title),
		SpaceID:         strings.TrimSpace(req.SpaceID),
		ModelProfileID:  strings.TrimSpace(req.ModelProfileID),
		Model:           strings.TrimSpace(req.Model),
		ReasoningEffort: strings.TrimSpace(req.ReasoningEffort),
		ExecutorTarget:  target,
		GitRepo:         strings.TrimSpace(req.GitRepo),
		GitBaseBranch:   strings.TrimSpace(req.GitBaseBranch),
		GitBranch:       strings.TrimSpace(req.GitBranch),
	})
	if err != nil {
		cloudapi.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionview.Public(session))
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
	surface := strings.TrimSpace(req.ArtifactSurface)
	if surface != "" && surface != string(visualize.SurfaceChat) && surface != string(visualize.SurfaceCanvas) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported artifact surface %q; use chat or canvas", surface))
		return
	}
	ctx, cancel := serverACPBootstrapContext()
	defer cancel()
	session, err := s.ACP.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:        strings.TrimSpace(req.Agent),
		Slug:            req.Slug,
		Title:           req.Title,
		Directory:       directory,
		Worktree:        req.Worktree,
		WorkspaceID:     s.activeWorkspaceID(),
		ArtifactSurface: surface,
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
