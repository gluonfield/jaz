package modelcapabilities

import (
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/modelcatalog"
)

type Service interface {
	ProviderModels(providerID string) ([]modelcatalog.Model, error)
}

type Handler struct {
	Service Service
}

type providerModelsResponse struct {
	Models []acp.AgentModel `json:"models"`
}

func NewHandler(service Service) Handler {
	return Handler{Service: service}
}

func (h Handler) ProviderModels(w http.ResponseWriter, r *http.Request) {
	models, err := acp.ProviderModelCapabilities(h.Service, r.URL.Query().Get("agent"), r.PathValue("provider"))
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
