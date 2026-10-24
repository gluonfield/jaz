package modelcapabilities

import (
	"context"
	"net/http"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type AgentModelSource interface {
	DiscoverModels(context.Context, string) (acp.AgentModelCatalog, error)
}

func NewAgentModelsHandler(source AgentModelSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalog, err := source.DiscoverModels(r.Context(), r.PathValue("agent"))
		if err != nil {
			httpapi.WriteError(w, http.StatusBadGateway, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, catalog)
	})
}
