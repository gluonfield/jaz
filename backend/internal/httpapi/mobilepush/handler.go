package mobilepush

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	var input mobilepush.Registration
	if !decode(w, r, &input) {
		return
	}
	id, ok := requireInstallationID(w, input.InstallationID)
	if !ok {
		return
	}
	writeResult(w, h.Service.Register(id, input))
}

func (h Handler) RegisterActivity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		mobilepush.Registration
		SessionID string `json:"session_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	id, ok := requireInstallationID(w, input.InstallationID)
	if !ok {
		return
	}
	writeResult(w, h.Service.RegisterActivity(id, mobilepush.Activity{
		Registration: input.Registration,
		SessionID:    input.SessionID,
		ID:           r.PathValue("activity"),
	}))
}

func (h Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	id, ok := requireInstallationID(w, r.URL.Query().Get("installation_id"))
	if !ok {
		return
	}
	writeResult(w, h.Service.Unregister(id, r.PathValue("activity")))
}

func requireInstallationID(w http.ResponseWriter, raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	if id == "" || len(id) > 128 {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("%w: installation_id is required", mobilepush.ErrInvalidRegistration))
		return "", false
	}
	return id, true
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
