package agentsessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

type Runtime interface {
	SetSessionConfig(context.Context, string, string, string) error
	StopBackgroundTask(context.Context, string, string) error
	Input(context.Context, acp.SteerRequest) (acp.Job, error)
}

type Handler struct{ runtime Runtime }

func NewHandler(runtime Runtime) *Handler {
	return &Handler{runtime: runtime}
}

func (h *Handler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID    string  `json:"id"`
		Value *string `json:"value"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" || input.Value == nil {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("config id and value are required"))
		return
	}
	if err := h.runtime.SetSessionConfig(r.Context(), r.PathValue("session"), input.ID, *input.Value); err != nil {
		httpapi.WriteError(w, http.StatusConflict, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, struct{}{})
}

func (h *Handler) StopTask(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.StopBackgroundTask(r.Context(), r.PathValue("session"), r.PathValue("task")); err != nil {
		httpapi.WriteError(w, http.StatusConflict, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, struct{}{})
}

func (h *Handler) Input(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Message  string                   `json:"message"`
		Contexts []storage.MessageContext `json:"contexts,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("message is required"))
		return
	}
	_, err := h.runtime.Input(r.Context(), acp.SteerRequest{Session: r.PathValue("session"), Message: input.Message, Contexts: input.Contexts})
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, acp.ErrSteeringUnsupported) {
			status = http.StatusNotImplemented
			err = errors.New("This agent cannot accept live follow-ups while it is working.")
		}
		httpapi.WriteError(w, status, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, struct{}{})
}
