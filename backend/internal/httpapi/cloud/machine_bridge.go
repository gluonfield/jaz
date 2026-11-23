package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/machinebridge"
)

var errMachineBridgeUnavailable = errors.New("machine bridge is unavailable")

func (h *Handler) MachineBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if h.machineBridge == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errMachineBridgeUnavailable)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.machineBridge.Status())
}

func (h *Handler) UpdateMachineBridge(w http.ResponseWriter, r *http.Request) {
	if h.machineBridge == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errMachineBridgeUnavailable)
		return
	}
	var cfg machinebridge.ShareConfig
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes))
	if err := dec.Decode(&cfg); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("request body must contain a single JSON object"))
		return
	}
	status, err := h.machineBridge.UpdateConfig(r.Context(), cfg)
	if err != nil {
		if errors.Is(err, machinebridge.ErrInvalidRequest) {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}
