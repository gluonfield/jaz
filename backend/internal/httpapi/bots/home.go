package bots

import (
	"errors"
	"net/http"

	botcore "github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type filesResponse struct {
	Files []botcore.File `json:"files"`
}

type saveFileRequest struct {
	Content string `json:"content"`
	Version string `json:"version"`
}

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("bot")
	if _, err := h.bots.LoadForActiveWorkspace(id); err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	files, err := h.bots.Files(id)
	if err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, filesResponse{Files: files})
}

// SaveFile answers a save over a stale version with 409 and the file as it
// now is.
func (h *Handler) SaveFile(w http.ResponseWriter, r *http.Request) {
	var input saveFileRequest
	if !decode(w, r, &input) {
		return
	}
	id := r.PathValue("bot")
	if _, err := h.bots.LoadForActiveWorkspace(id); err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	file, err := h.bots.SaveFile(id, r.PathValue("name"), input.Content, input.Version)
	switch {
	case errors.Is(err, botcore.ErrFileChanged):
		httpapi.WriteJSON(w, http.StatusConflict, file)
	case err != nil:
		httpapi.WriteError(w, errorStatus(err), err)
	default:
		httpapi.WriteJSON(w, http.StatusOK, file)
	}
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("bot")
	if _, err := h.bots.LoadForActiveWorkspace(id); err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	view, err := h.bots.Activity(id)
	if err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, view)
}
