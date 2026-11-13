package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

const (
	maxPatchBytes  = 4 << 20
	maxUploadBytes = 128 << 20
)

type SessionLoader interface {
	LoadSession(ref string) (storage.Session, error)
}

type Handler struct {
	sessions SessionLoader
	canvas   *canvas.Service
}

func NewHandler(sessions SessionLoader, service *canvas.Service) *Handler {
	return &Handler{sessions: sessions, canvas: service}
}

// Get answers 204 when the board still matches the version the client holds,
// so polling during a turn costs a stat per file rather than a compile.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.canvasSession(w, r)
	if !ok {
		return
	}
	snapshot, err := h.canvas.Load(id)
	if err != nil {
		fail(w, err)
		return
	}
	if r.URL.Query().Get("version") == snapshot.Version {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, snapshot.View())
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := h.canvasSession(w, r)
	if !ok {
		return
	}
	var patch canvas.Patch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPatchBytes)).Decode(&patch); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	version, err := h.canvas.Patch(id, patch)
	if err != nil {
		fail(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"version": version})
}

func (h *Handler) PutLayout(w http.ResponseWriter, r *http.Request) {
	id, ok := h.canvasSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Page string `json:"page"`
		canvas.Layout
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPatchBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.canvas.SaveLayout(id, body.Page, body.Layout); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PostFile takes one multipart `file` and answers with its path in the canvas
// folder, ready to be a file node.
func (h *Handler) PostFile(w http.ResponseWriter, r *http.Request) {
	id, ok := h.canvasSession(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	parts, err := r.MultipartReader()
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	for {
		part, err := parts.NextPart()
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("file is required"))
			return
		}
		if part.FormName() != "file" {
			continue
		}
		path, err := h.canvas.SaveUpload(id, part.FileName(), part)
		if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
			httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"file": path})
		return
	}
}

func (h *Handler) canvasSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	session, err := h.sessions.LoadSession(r.PathValue("session"))
	if errors.Is(err, storage.ErrSessionNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, err)
		return "", false
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return "", false
	}
	if session.RuntimeRef == nil || visualize.NormalizeSurface(session.RuntimeRef.ArtifactSurface) != visualize.SurfaceCanvas {
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("session %s is not a canvas thread", session.ID))
		return "", false
	}
	return session.ID, true
}

func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, canvas.ErrInvalid) {
		status = http.StatusUnprocessableEntity
	}
	httpapi.WriteError(w, status, err)
}
