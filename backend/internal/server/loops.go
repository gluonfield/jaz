package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
)

func (s *Server) handleListLoops(w http.ResponseWriter, r *http.Request) {
	if s.Loops == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("loops are not configured"))
		return
	}
	items, err := s.Loops.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"loops": items})
}

func (s *Server) handleCreateLoop(w http.ResponseWriter, r *http.Request) {
	if s.Loops == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("loops are not configured"))
		return
	}
	var req loopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// No threadID: the REST surface has no thread, so it never emits a card.
	loop, err := s.loopCoordinator().Create(loops.CreateLoop{
		Name:            req.Name,
		Prompt:          req.Prompt,
		Schedule:        req.Schedule,
		Status:          req.Status,
		Runtime:         req.Runtime,
		ACPAgent:        req.ACPAgent,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Directory:       req.Directory,
	}, req.BoardIDs, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, loop)
}

// loopCoordinator builds the shared create/update use case for the REST surface.
// Boards come from the widget service; there is no card sink because REST
// requests carry no thread to announce into.
func (s *Server) loopCoordinator() loops.Coordinator {
	coordinator := loops.Coordinator{Loops: s.Loops}
	if s.Widgets != nil {
		coordinator.Boards = s.Widgets.LoopBoards()
	}
	return coordinator
}

func (s *Server) handleLoopAction(w http.ResponseWriter, r *http.Request) {
	if s.Loops == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("loops are not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/loops/")
	loopID, action, hasAction := strings.Cut(rest, "/")
	if strings.TrimSpace(loopID) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("loop id is required"))
		return
	}
	if !hasAction {
		switch r.Method {
		case http.MethodGet:
			s.handleGetLoop(w, r, loopID)
		case http.MethodPatch:
			s.handlePatchLoop(w, r, loopID)
		case http.MethodDelete:
			if err := s.Loops.Delete(loopID); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			// The loop's widget dies with it; a lingering placement would
			// invisibly occupy board cells.
			if s.Widgets != nil {
				s.Widgets.PurgeOrphans()
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		}
		return
	}
	switch {
	case action == "runs" && r.Method == http.MethodGet:
		s.handleListLoopRuns(w, r, loopID)
	case action == "run" && r.Method == http.MethodPost:
		run, err := s.Loops.RunNow(r.Context(), loopID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func (s *Server) handleGetLoop(w http.ResponseWriter, _ *http.Request, loopID string) {
	loop, err := s.Loops.Load(loopID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	runs, err := s.Loops.Runs(loopID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	boardIDs := []string{}
	if s.Widgets != nil {
		if _, boards, found, err := s.Widgets.StateForLoop(loopID); err == nil && found {
			boardIDs = boards
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"loop": loop, "runs": runs, "board_ids": boardIDs})
}

func (s *Server) handlePatchLoop(w http.ResponseWriter, r *http.Request, loopID string) {
	var req patchLoopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	loop, err := s.loopCoordinator().Update(loopID, loops.UpdateLoop{
		Name:            req.Name,
		Prompt:          req.Prompt,
		Schedule:        req.Schedule,
		Status:          req.Status,
		Runtime:         req.Runtime,
		ACPAgent:        req.ACPAgent,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Directory:       req.Directory,
	}, req.BoardIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, loop)
}

func (s *Server) handleListLoopRuns(w http.ResponseWriter, r *http.Request, loopID string) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	runs, err := s.Loops.Runs(loopID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

type loopRequest struct {
	Name            string         `json:"name,omitempty"`
	Prompt          string         `json:"prompt"`
	Schedule        loops.Schedule `json:"schedule"`
	Status          string         `json:"status,omitempty"`
	Runtime         string         `json:"runtime,omitempty"`
	ACPAgent        string         `json:"acp_agent,omitempty"`
	ModelProvider   string         `json:"model_provider,omitempty"`
	Model           string         `json:"model,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Directory       string         `json:"directory,omitempty"`
	// BoardIDs assigns the loop's widget to boards; assignment is what enables
	// widget publishing (there is no separate toggle).
	BoardIDs []string `json:"board_ids,omitempty"`
}

type patchLoopRequest struct {
	Name            *string         `json:"name,omitempty"`
	Prompt          *string         `json:"prompt,omitempty"`
	Schedule        *loops.Schedule `json:"schedule,omitempty"`
	Status          *string         `json:"status,omitempty"`
	Runtime         *string         `json:"runtime,omitempty"`
	ACPAgent        *string         `json:"acp_agent,omitempty"`
	ModelProvider   *string         `json:"model_provider,omitempty"`
	Model           *string         `json:"model,omitempty"`
	ReasoningEffort *string         `json:"reasoning_effort,omitempty"`
	Directory       *string         `json:"directory,omitempty"`
	BoardIDs        *[]string       `json:"board_ids,omitempty"`
}
