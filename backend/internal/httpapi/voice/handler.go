package voice

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/voice/live"
)

type Handler struct {
	service    *live.Service
	transcript *live.Transcript
}

func NewHandler(service *live.Service, transcript *live.Transcript) *Handler {
	return &Handler{service: service, transcript: transcript}
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	var status live.Status
	var err error
	if r.Method == http.MethodPut {
		var input live.Settings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		status, err = h.service.SaveSettings(input)
	} else {
		status, err = h.service.Settings()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SDP     string `json:"sdp"`
		Context string `json:"context"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	connection, err := h.service.Connect(r.Context(), input.SDP, input.Context)
	if err != nil {
		writeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, connection)
}

func (h *Handler) Transcript(w http.ResponseWriter, r *http.Request) {
	var messages []sessionevents.VoiceMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&messages); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.transcript.Append(r.PathValue("session"), messages); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, storage.ErrSessionNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, live.ErrInput) {
		status = http.StatusBadRequest
	} else if errors.Is(err, live.ErrUnavailable) {
		status = http.StatusServiceUnavailable
	}
	httpapi.WriteError(w, status, err)
}
