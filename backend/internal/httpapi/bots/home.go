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

type activityResponse struct {
	Items []botcore.Activity `json:"items"`
	Owed  []botcore.Owed     `json:"owed"`
}

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	files, err := h.bots.Files(r.PathValue("bot"))
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
	file, err := h.bots.SaveFile(r.PathValue("bot"), r.PathValue("name"), input.Content, input.Version)
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
	items, err := h.bots.Activity(r.PathValue("bot"))
	if err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	owed, err := h.bots.Owed(r.PathValue("bot"))
	if err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, activityResponse{Items: items, Owed: owed})
}
