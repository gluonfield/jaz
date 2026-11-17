// Package plugins serves the installed plugins to Settings.
package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/storage"
)

const maxArchiveBytes = 64 << 20

type MCPRuntime interface {
	RefreshServersAsync(...string)
}

type Handler struct {
	runtime  MCPRuntime
	store    *plugins.Store
	sessions plugins.SessionStore
}

type listResponse struct {
	Plugins []plugins.Plugin `json:"plugins"`
}

func NewHandler(sessions plugins.SessionStore, runtime MCPRuntime, store *plugins.Store) *Handler {
	return &Handler{store: store, sessions: sessions, runtime: runtime}
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
	h.writeInstalled(w, plugins.InstallResult{Plugins: installed}, err)
}

func (h *Handler) InstallRepository(w http.ResponseWriter, r *http.Request) {
	var input plugins.RepositoryInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.store.InstallRepository(r.Context(), input)
	h.writeInstalled(w, result, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	result, err := h.store.Update(r.Context(), r.PathValue("plugin"))
	h.writeInstalled(w, result, err)
}

func (h *Handler) writeInstalled(w http.ResponseWriter, result plugins.InstallResult, err error) {
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
	var ids []string
	for _, plugin := range result.Plugins {
		for _, name := range plugin.MCPServers {
			ids = append(ids, plugins.MCPServerID(plugin.Name, name))
		}
	}
	h.runtime.RefreshServersAsync(ids...)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Remove(r.PathValue("plugin")); err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	h.runtime.RefreshServersAsync()
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, plugins.ErrNotFound), errors.Is(err, storage.ErrSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, plugins.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
