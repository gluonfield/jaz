package providers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/modelcatalog"
)

type CatalogHandler struct{ discovery modelcatalog.Discovery }

func NewCatalogHandler(discovery modelcatalog.Discovery) *CatalogHandler {
	return &CatalogHandler{discovery: discovery}
}

func (h *CatalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID        string            `json:"id"`
		BaseURL   string            `json:"base_url"`
		ModelsURL string            `json:"models_url"`
		APIKey    string            `json:"api_key"`
		Headers   map[string]string `json:"headers"`
		Save      bool              `json:"save"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.discovery.Fetch(r.Context(), modelcatalog.DiscoveryInput{ProviderID: input.ID, BaseURL: input.BaseURL, ModelsURL: input.ModelsURL, APIKey: input.APIKey, Headers: input.Headers, Save: input.Save})
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, modelcatalog.ErrDiscoveryInput):
			status = http.StatusBadRequest
		case errors.Is(err, modelcatalog.ErrDiscoveryProvider):
			status = http.StatusNotFound
		case errors.Is(err, modelcatalog.ErrCatalogUnavailable):
			status = http.StatusBadGateway
		}
		httpapi.WriteError(w, status, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, struct {
		Models     []string `json:"models"`
		CatalogURL string   `json:"catalog_url"`
		Saved      bool     `json:"saved"`
	}{result.Models, result.CatalogURL, result.Saved})
}
