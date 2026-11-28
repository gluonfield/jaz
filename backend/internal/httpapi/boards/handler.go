// Package boards serves a board's layout: the tiles of an app of kind board.
// The board itself is listed, created, renamed, shared and deleted as an app.
package boards

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	boardscore "github.com/wins/jaz/backend/internal/boards"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

type CloudProxy interface {
	ProxyActiveWorkspace(w http.ResponseWriter, r *http.Request) bool
}

type LoopSource interface {
	LoadForActiveWorkspace(id string) (loops.Loop, error)
}

type Handler struct {
	service *boardscore.Service
	loops   LoopSource
	cloud   CloudProxy
}

func NewHandler(service *boardscore.Service, loops LoopSource, cloud CloudProxy) *Handler {
	return &Handler{service: service, loops: loops, cloud: cloud}
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, storage.ErrAppNotFound) {
		status = http.StatusNotFound
	}
	httpapi.WriteError(w, status, err)
}

func decode(w http.ResponseWriter, r *http.Request, body any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return false
	}
	return true
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	view, err := h.service.View(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, view)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	if h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	var update boardscore.Update
	if !decode(w, r, &update) {
		return
	}
	if err := h.service.Patch(r.Context(), r.PathValue("id"), update); err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AddTiles puts apps, and the apps of loops, on the board. Being on a board
// is what makes a loop publish an app.
func (h *Handler) AddTiles(w http.ResponseWriter, r *http.Request) {
	if h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	var body struct {
		AppIDs  []string `json:"app_ids"`
		LoopIDs []string `json:"loop_ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	if count := len(body.AppIDs) + len(body.LoopIDs); count == 0 || count > 50 {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("app_ids and loop_ids must name between 1 and 50 tiles"))
		return
	}
	tileLoops := make([]loops.Loop, 0, len(body.LoopIDs))
	for _, loopID := range body.LoopIDs {
		loop, err := h.loops.LoadForActiveWorkspace(loopID)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("unknown loop %s", loopID))
			return
		}
		tileLoops = append(tileLoops, loop)
	}
	if err := h.service.AddTiles(r.Context(), r.PathValue("id"), body.AppIDs, tileLoops); err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) RemoveTile(w http.ResponseWriter, r *http.Request) {
	if h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	if err := h.service.RemoveTile(r.Context(), r.PathValue("id"), r.PathValue("app")); err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// TileFeedback records what a board measured while showing the app: a render
// error, or layout telemetry the app's loop reads on its next run.
func (h *Handler) TileFeedback(w http.ResponseWriter, r *http.Request) {
	if h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	var body struct {
		Error  string          `json:"error"`
		Layout json.RawMessage `json:"layout"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := h.service.ReportTile(r.Context(), r.PathValue("id"), boardscore.Feedback{Error: body.Error, Layout: string(body.Layout)}); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
