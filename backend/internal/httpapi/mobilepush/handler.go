package mobilepush

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mobilepush"
)

type Handler struct {
	Service *mobilepush.Service
}

func (h Handler) Status(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"configured": h.Service.Enabled()})
}

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := authenticatedDevice(w, r)
	if !ok {
		return
	}
	var input mobilepush.Registration
	if !decode(w, r, &input) {
		return
	}
	writeResult(w, h.Service.Register(deviceID, input))
}

func (h Handler) RegisterActivity(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := authenticatedDevice(w, r)
	if !ok {
		return
	}
	var input struct {
		mobilepush.Registration
		SessionID string `json:"session_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	writeResult(w, h.Service.RegisterActivity(deviceID, mobilepush.Activity{
		Registration: input.Registration,
		SessionID:    input.SessionID,
		ID:           r.PathValue("activity"),
	}))
}

func (h Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := authenticatedDevice(w, r)
	if !ok {
		return
	}
	writeResult(w, h.Service.Unregister(deviceID, r.PathValue("activity")))
}

func authenticatedDevice(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := deviceauth.PrincipalFromContext(r.Context())
	if !ok || principal.Kind != deviceauth.PrincipalDevice || principal.DeviceID == "" {
		httpapi.WriteError(w, http.StatusForbidden, errors.New("pair this device before enabling notifications"))
		return "", false
	}
	return principal.DeviceID, true
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(target); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, mobilepush.ErrInvalidRegistration) {
			status = http.StatusBadRequest
		}
		httpapi.WriteError(w, status, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
