package mobilepush

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mobilepush"
)

const defaultPushClientID = "default"

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
	writeResult(w, h.Service.Register(pushClientID(r, input.InstallationID), input))
}

func (h Handler) RegisterActivity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		mobilepush.Registration
		SessionID string `json:"session_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	writeResult(w, h.Service.RegisterActivity(pushClientID(r, input.InstallationID), mobilepush.Activity{
		Registration: input.Registration,
		SessionID:    input.SessionID,
		ID:           r.PathValue("activity"),
	}))
}

func (h Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	writeResult(w, h.Service.Unregister(pushClientID(r, r.URL.Query().Get("installation_id")), r.PathValue("activity")))
}

func pushClientID(r *http.Request, installationID string) string {
	for _, candidate := range []string{
		r.Header.Get("X-Jaz-Device-ID"),
		installationID,
	} {
		if id := strings.TrimSpace(candidate); id != "" && len(id) <= 128 {
			return id
		}
	}
	return defaultPushClientID
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
