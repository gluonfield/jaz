// Package plugins serves the installed plugins to Settings.
package plugins

import (
	"errors"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/plugins"
)

const maxArchiveBytes = 64 << 20

type Handler struct {
	store    *plugins.Store
	sessions plugins.SessionStore
}

type listResponse struct {
	Plugins []plugins.Plugin `json:"plugins"`
}

func NewHandler(store *plugins.Store, sessions plugins.SessionStore) *Handler {
	return &Handler{store: store, sessions: sessions}
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	page, err := h.store.Page(r.PathValue("plugin"), h.sessions)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) List(w http.ResponseWriter, _ *http.Request) {
	installed, err := h.store.List()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, listResponse{Plugins: installed})
}

// Install takes a zip as the request body and adds every plugin in it.
func (h *Handler) Install(w http.ResponseWriter, r *http.Request) {
	archive, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxArchiveBytes))
	if err != nil {
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	installed, err := h.store.Install(archive)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, listResponse{Plugins: installed})
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Remove(r.PathValue("plugin")); err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, plugins.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, plugins.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
