package connections

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type PluginHandler struct {
	Service *connections.Service
}

func NewPluginHandler(service *connections.Service) PluginHandler {
	return PluginHandler{Service: service}
}

func (h PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	plugins, err := h.Service.ListPlugins(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
}

func (h PluginHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	plugin, ok, err := h.Service.Plugin(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		httpapi.WriteError(w, http.StatusNotFound, errors.New("connection plugin not found"))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, plugin)
}

func (h PluginHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.Service.DisconnectAccount(r.Context(), id); err != nil {
		if errors.Is(err, connections.ErrConnectionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type updateScopesRequest struct {
	Scopes []string `json:"scopes"`
}

func (h PluginHandler) UpdateScopes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var input updateScopesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	account, err := h.Service.UpdateAccountScopes(r.Context(), id, input.Scopes)
	if err != nil {
		if errors.Is(err, connections.ErrConnectionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, account)
}
