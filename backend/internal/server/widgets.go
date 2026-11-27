package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/widgets"
)

func (s *Server) handleListBoards(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	boards, err := s.Widgets.ListBoards()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
}

func (s *Server) handleCreateBoard(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board, err := s.Widgets.CreateBoard(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (s *Server) handleBoardAction(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/boards/")
	boardID, sub, hasSub := strings.Cut(rest, "/")
	if strings.TrimSpace(boardID) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("board id is required"))
		return
	}
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	if hasSub {
		if widgetID, ok := strings.CutPrefix(sub, "widgets/"); ok && r.Method == http.MethodDelete {
			if err := s.Widgets.RemoveFromBoard(boardID, widgetID); err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, widgets.ErrNotFound) {
					status = http.StatusNotFound
				}
				writeError(w, status, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if sub == "loops" && r.Method == http.MethodPost {
			s.handleAssignLoopsToBoard(w, r, boardID)
			return
		}
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetBoard(w, boardID)
	case http.MethodPatch:
		s.handlePatchBoard(w, r, boardID)
	case http.MethodDelete:
		if err := s.Widgets.DeleteBoard(boardID); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, widgets.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

// handleAssignLoopsToBoard adds loops to a board additively — the onboarding
// flow's final step. Assignment is what enables a loop's widget.
func (s *Server) handleAssignLoopsToBoard(w http.ResponseWriter, r *http.Request, boardID string) {
	if s.Loops == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("loops are not configured"))
		return
	}
	if _, err := s.Widgets.LoadBoard(boardID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req struct {
		LoopIDs []string `json:"loop_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	for _, loopID := range req.LoopIDs {
		loop, err := s.Loops.LoadForActiveWorkspace(loopID)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("unknown loop %s", loopID))
			return
		}
		if _, err := s.Widgets.AddLoopToBoard(loop, boardID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetBoard(w http.ResponseWriter, boardID string) {
	board, err := s.Widgets.LoadBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	items, err := s.Widgets.ListBoardItems(boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"board": board, "items": items})
}

func (s *Server) handlePatchBoard(w http.ResponseWriter, r *http.Request, boardID string) {
	var req struct {
		Name         *string               `json:"name,omitempty"`
		WindowBounds *string               `json:"window_bounds,omitempty"`
		FontScale    *float64              `json:"font_scale,omitempty"`
		Layout       []widgets.LayoutEntry `json:"layout,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board, err := s.Widgets.PatchBoard(boardID, widgets.UpdateBoard{
		Name:         req.Name,
		WindowBounds: req.WindowBounds,
		FontScale:    req.FontScale,
		Layout:       req.Layout,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, widgets.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (s *Server) handleWidgetAction(w http.ResponseWriter, r *http.Request) {
	if s.proxyCloud(w, r) {
		return
	}
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/widgets/")
	widgetID, action, hasAction := strings.Cut(rest, "/")
	if strings.TrimSpace(widgetID) == "" || !hasAction {
		writeError(w, http.StatusBadRequest, fmt.Errorf("widget id and action are required"))
		return
	}
	switch {
	case action == "errors" && r.Method == http.MethodPost:
		if _, err := s.Widgets.LoadWidget(widgetID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.Widgets.ReportError(widgetID, req.Message); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case action == "layout" && r.Method == http.MethodPost:
		if _, err := s.Widgets.LoadWidget(widgetID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		// Board-measured layout telemetry (dead space, overflow, clipping);
		// stored raw and surfaced in the loop's next-run prompt.
		payload, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.Widgets.ReportLayout(widgetID, string(payload)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}
