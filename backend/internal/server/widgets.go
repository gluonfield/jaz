package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/widgets"
)

type cloudBoardsBridge interface {
	ListBoards(context.Context) (json.RawMessage, error)
	CreateBoard(context.Context, json.RawMessage) (json.RawMessage, int, error)
	GetBoard(context.Context, string) (json.RawMessage, int, error)
	PatchBoard(context.Context, string, json.RawMessage) (json.RawMessage, int, error)
	DeleteBoard(context.Context, string) (json.RawMessage, int, error)
	DeleteBoardWidget(context.Context, string, string) (json.RawMessage, int, error)
}

func (s *Server) cloudBoards() cloudBoardsBridge {
	if s.activeWorkspaceID() == storage.LocalWorkspaceID || s.Cloud == nil {
		return nil
	}
	cb, ok := s.Cloud.(cloudBoardsBridge)
	if !ok {
		return nil
	}
	return cb
}

func (s *Server) handleListBoards(w http.ResponseWriter, r *http.Request) {
	if cb := s.cloudBoards(); cb != nil {
		raw, err := cb.ListBoards(r.Context())
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, http.StatusOK, raw)
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
	if cb := s.cloudBoards(); cb != nil {
		body, err := cloudapi.ReadLimitedBody(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		raw, status, err := cb.CreateBoard(r.Context(), body)
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, status, raw)
		return
	}
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	rest := strings.TrimPrefix(r.URL.Path, "/v1/boards/")
	boardID, sub, hasSub := strings.Cut(rest, "/")
	if strings.TrimSpace(boardID) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("board id is required"))
		return
	}
	if cb := s.cloudBoards(); cb != nil {
		s.handleCloudBoardAction(w, r, cb, boardID, sub, hasSub)
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

func (s *Server) handleCloudBoardAction(w http.ResponseWriter, r *http.Request, cb cloudBoardsBridge, boardID, sub string, hasSub bool) {
	if hasSub {
		if widgetID, ok := strings.CutPrefix(sub, "widgets/"); ok && r.Method == http.MethodDelete {
			raw, status, err := cb.DeleteBoardWidget(r.Context(), boardID, widgetID)
			if err != nil {
				cloudapi.WriteError(w, err)
				return
			}
			cloudapi.WriteRawJSON(w, status, raw)
			return
		}
		if sub == "loops" && r.Method == http.MethodPost {
			writeError(w, http.StatusNotImplemented, fmt.Errorf("assigning local loops to cloud boards is not supported"))
			return
		}
		writeError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		raw, status, err := cb.GetBoard(r.Context(), boardID)
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, status, raw)
	case http.MethodPatch:
		body, err := cloudapi.ReadLimitedBody(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		raw, status, err := cb.PatchBoard(r.Context(), boardID, body)
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, status, raw)
	case http.MethodDelete:
		raw, status, err := cb.DeleteBoard(r.Context(), boardID)
		if err != nil {
			cloudapi.WriteError(w, err)
			return
		}
		cloudapi.WriteRawJSON(w, status, raw)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

func (s *Server) handleListWidgets(w http.ResponseWriter, _ *http.Request) {
	if s.Widgets == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("widgets are not configured"))
		return
	}
	items, err := s.Widgets.ListWidgets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"widgets": items})
}

func (s *Server) handleWidgetAction(w http.ResponseWriter, r *http.Request) {
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
	case action == "content" && r.Method == http.MethodGet:
		s.handleWidgetContent(w, r, widgetID)
	case action == "errors" && r.Method == http.MethodPost:
		if _, err := s.Widgets.LoadWidget(widgetID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

func (s *Server) handleWidgetContent(w http.ResponseWriter, r *http.Request, widgetID string) {
	widget, err := s.Widgets.LoadWidget(widgetID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	version := widget.CurrentVersion
	if raw := strings.TrimSpace(r.URL.Query().Get("version")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			version = v
		}
	}
	if version == 0 {
		writeError(w, http.StatusNotFound, fmt.Errorf("widget %s has no published version", widgetID))
		return
	}
	snapshot, err := s.Widgets.Repo.LoadWidgetVersion(widget.ID, version)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	// Serve the raw fragment. The board wraps it in the shared artifact document
	// (theme, color-scheme, CSP, bridge) client-side, identical to inline
	// artifacts — so theme and zoom are applied by the host, not baked here.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(snapshot.HTML))
}
