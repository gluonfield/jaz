package modelcatalog

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	catalog "github.com/wins/jaz/backend/internal/modelcatalog"
)

type Service interface {
	ProviderModelsForAgent(agent, providerID string) ([]catalog.Model, error)
}

type Handler struct {
	Service Service
}

type providerModelsResponse struct {
	Models []catalog.Model `json:"models"`
}

func NewHandler(service Service) Handler {
	return Handler{Service: service}
}

func (h Handler) ProviderModels(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	agent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent")))
	models, err := h.Service.ProviderModelsForAgent(agent, id)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, catalog.ErrCatalogUnavailable) {
			status = http.StatusServiceUnavailable
		}
		httpapi.WriteError(w, status, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, providerModelsResponse{Models: models})
}
