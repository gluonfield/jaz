package modelcapabilities

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

type Handler struct {
	Service acp.ModelCatalog
	// NativeSource serves agent-discovered catalogs, e.g. OpenCode accounts,
	// whose models live in the agent rather than a provider endpoint.
	NativeSource AgentModelSource
}

type providerModelsResponse struct {
	Models []acp.AgentModel `json:"models"`
}

func NewHandler(service acp.ModelCatalog) Handler {
	return Handler{Service: service}
}

func (h Handler) ProviderModels(w http.ResponseWriter, r *http.Request) {
	agentName := r.URL.Query().Get("agent")
	providerID := r.PathValue("provider")
	if h.NativeSource != nil && strings.EqualFold(strings.TrimSpace(agentName), "opencode") &&
		strings.EqualFold(strings.TrimSpace(providerID), provider.ProviderOpenCodeNative) {
		catalog, err := h.NativeSource.DiscoverModels(r.Context(), agentName)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadGateway, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, providerModelsResponse{Models: catalog.Models})
		return
	}
	models, err := (acp.ModelCapabilities{Catalog: h.Service}).ProviderModels(agentName, providerID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
			status = http.StatusServiceUnavailable
		}
		httpapi.WriteError(w, status, err)
		return
	}
	for _, model := range models {
		if model.Reasoning.Status == modelcatalog.ReasoningPending {
			httpapi.WriteError(w, http.StatusServiceUnavailable, modelcatalog.ErrCatalogUnavailable)
			return
		}
	}
	httpapi.WriteJSON(w, http.StatusOK, providerModelsResponse{Models: models})
}
