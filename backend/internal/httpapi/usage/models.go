package usage

import (
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

type modelsHandler struct {
	service usagecore.Service
}

type modelsResponse struct {
	Models []modelUsageDTO `json:"models"`
}

type modelUsageDTO struct {
	Agent         string         `json:"agent,omitempty"`
	ModelProvider string         `json:"model_provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	Usage         usageTotalsDTO `json:"usage"`
	SessionCount  int            `json:"session_count"`
}

func NewModelsHandler(service usagecore.Service) http.Handler {
	return modelsHandler{service: service}
}

func (h modelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query, err := parseDailyQuery(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	models, err := h.service.Models(query)
	switch {
	case errors.Is(err, usagecore.ErrUnsupported):
		httpapi.WriteError(w, http.StatusNotImplemented, err)
	case errors.Is(err, usagecore.ErrInvalidDays):
		httpapi.WriteError(w, http.StatusBadRequest, err)
	case err != nil:
		httpapi.WriteError(w, http.StatusInternalServerError, err)
	default:
		httpapi.WriteJSON(w, http.StatusOK, modelsResponse{Models: modelDTOs(models)})
	}
}

func modelDTOs(models []usagecore.ModelUsage) []modelUsageDTO {
	out := make([]modelUsageDTO, len(models))
	for i, model := range models {
		out[i] = modelUsageDTO{
			Agent:         model.Agent,
			ModelProvider: model.ModelProvider,
			Model:         model.Model,
			Usage:         usageDTO(model.Usage),
			SessionCount:  model.SessionCount,
		}
	}
	return out
}
