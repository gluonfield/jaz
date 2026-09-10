package agentsessions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
)

type Runtime interface {
	SetSessionConfig(context.Context, string, string, string) error
	StopBackgroundTask(context.Context, string, string) error
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
