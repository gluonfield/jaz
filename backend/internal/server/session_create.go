package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/provider"
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
	if req.Runtime == storage.RuntimeACP && strings.TrimSpace(req.Agent) != "" {
		s.createACPSession(w, req)
		return
	}
	input, err := s.nativeSessionDefaults()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if requested := strings.TrimSpace(req.ModelProvider); requested != "" {
		id, err := provider.NormalizeNativeProviderID(requested)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if id != input.ModelProvider {
			meta, _ := provider.NativeProviderByID(id)
			input.Model = strings.TrimSpace(meta.DefaultModel)
		}
		input.ModelProvider = id
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		input.Model = model
	}
	if effort := strings.TrimSpace(req.ReasoningEffort); effort != "" {
		input.ReasoningEffort = effort
	}
	if err := s.validateNativeProviderRunnable(input.ModelProvider); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	directory := strings.TrimSpace(req.Directory)
	if req.Worktree && directory == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("worktree requires a directory pointing at a git repository"))
		return
	}
	ref, err := s.nativeRuntimeRef(directory)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	input.RuntimeRef = ref
	input.Slug = req.Slug
	input.Title = req.Title
	session, err := s.Store.CreateSession(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if req.Worktree {
		worktree, repo, err := gitinfo.AddWorktree(r.Context(), s.Workspace, session.RuntimeRef.Cwd, session.Slug, "")
		if err != nil {
			session.Status = storage.StatusError
			session.Error = err.Error()
			_ = s.Store.SaveSession(session)
			writeError(w, http.StatusBadRequest, err)
			return
		}
		session.RuntimeRef.Cwd = worktree
		session.RuntimeRef.ProjectPath = repo
		if err := s.Store.SaveSession(session); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
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
	writeJSON(w, http.StatusOK, canonicalSessionResponse(session))
}
