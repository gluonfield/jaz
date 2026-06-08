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
	loop, err := s.Loops.Create(loops.CreateLoop{
		Name:     req.Name,
		Prompt:   req.Prompt,
		Schedule: req.Schedule,
		Status:   req.Status,
		Runtime:  req.Runtime,
		ACPAgent: req.ACPAgent,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, loop)
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
	writeJSON(w, http.StatusOK, map[string]any{"loop": loop, "runs": runs})
}

func (s *Server) handlePatchLoop(w http.ResponseWriter, r *http.Request, loopID string) {
	var req patchLoopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	loop, err := s.Loops.Update(loopID, loops.UpdateLoop{
		Name:     req.Name,
		Prompt:   req.Prompt,
		Schedule: req.Schedule,
		Status:   req.Status,
		Runtime:  req.Runtime,
		ACPAgent: req.ACPAgent,
	})
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
	Name     string         `json:"name,omitempty"`
	Prompt   string         `json:"prompt"`
	Schedule loops.Schedule `json:"schedule"`
	Status   string         `json:"status,omitempty"`
	Runtime  string         `json:"runtime,omitempty"`
	ACPAgent string         `json:"acp_agent,omitempty"`
}

type patchLoopRequest struct {
	Name     *string         `json:"name,omitempty"`
	Prompt   *string         `json:"prompt,omitempty"`
	Schedule *loops.Schedule `json:"schedule,omitempty"`
	Status   *string         `json:"status,omitempty"`
	Runtime  *string         `json:"runtime,omitempty"`
	ACPAgent *string         `json:"acp_agent,omitempty"`
}
