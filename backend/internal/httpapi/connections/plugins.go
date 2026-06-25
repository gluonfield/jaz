package connections

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type PluginHandler struct {
	Catalog *connections.Catalog
}

func NewPluginHandler(catalog *connections.Catalog) PluginHandler {
	return PluginHandler{Catalog: catalog}
}

func (h PluginHandler) List(w http.ResponseWriter, _ *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"plugins": h.Catalog.ListPlugins()})
}

func (h PluginHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	plugin, ok := h.Catalog.Plugin(id)
	if !ok {
		httpapi.WriteError(w, http.StatusNotFound, errors.New("connection plugin not found"))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, plugin)
}
