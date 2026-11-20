package connections

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
	mcpapi "github.com/wins/jaz/backend/internal/httpapi/mcp"
)

type PluginHandler struct {
	Service *connections.Service
	MCP     MCPRefresher
	callbackBaseURL string
}

func NewPluginHandler(service *connections.Service, mcp MCPRefresher, callbackBaseURL string) PluginHandler {
	return PluginHandler{Service: service, MCP: mcp, callbackBaseURL: callbackBaseURL}
}

func (h PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	plugins, err := h.Service.ListPlugins(r.Context(), mcpapi.AuthorizeOptions(r, h.callbackBaseURL).LoopbackAllowed)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
}

func (h PluginHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	plugin, ok, err := h.Service.Plugin(r.Context(), id, mcpapi.AuthorizeOptions(r, h.callbackBaseURL).LoopbackAllowed)
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
	result, err := h.Service.DisconnectAccount(r.Context(), id)
	if err != nil {
		if errors.Is(err, connections.ErrConnectionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if result.MCPServersChanged {
		refreshMCP(h.MCP)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
