package sessions

import (
	"errors"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/sessionoverview"
	"github.com/wins/jaz/backend/internal/storage"
)

type OverviewHandler struct {
	service *sessionoverview.Service
}

type overviewResponse struct {
	Threads   []overviewThreadResponse   `json:"threads"`
	Subagents []overviewSubagentResponse `json:"subagents"`
}

type overviewThreadResponse struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Title           string    `json:"title,omitempty"`
	Agent           string    `json:"acp_agent"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	State           string    `json:"state"`
	Archived        bool      `json:"archived,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastEventAt     time.Time `json:"last_event_at,omitzero"`
}

type overviewSubagentResponse struct {
	Key             string    `json:"key"`
	Seq             int64     `json:"seq"`
	Provider        string    `json:"provider,omitempty"`
	ID              string    `json:"id"`
	Name            string    `json:"name,omitempty"`
	Task            string    `json:"task,omitempty"`
	Role            string    `json:"role,omitempty"`
	Status          string    `json:"status,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Prompt          string    `json:"prompt,omitempty"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewOverviewHandler(service *sessionoverview.Service) *OverviewHandler {
	return &OverviewHandler{service: service}
}

func (h *OverviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Load(r.Context(), r.PathValue("session"))
	if err != nil {
		if errors.Is(err, storage.ErrSessionNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	response := overviewResponse{
		Threads:   make([]overviewThreadResponse, 0, len(view.Threads)),
		Subagents: make([]overviewSubagentResponse, 0, len(view.Subagents)),
	}
	for _, thread := range view.Threads {
		response.Threads = append(response.Threads, overviewThreadResponse{
			ID: thread.ID, Slug: thread.Slug, Title: thread.Title, Agent: thread.Agent,
			Model: thread.Model, ReasoningEffort: thread.ReasoningEffort,
			State: thread.State, Archived: thread.Archived,
			UpdatedAt: thread.UpdatedAt, LastEventAt: thread.LastEventAt,
		})
	}
	for _, subagent := range view.Subagents {
		response.Subagents = append(response.Subagents, overviewSubagentResponse{
			Key: subagent.Key, Seq: subagent.Seq, Provider: subagent.Provider, ID: subagent.ID,
			Name: subagent.Name, Task: subagent.Task, Role: subagent.Role,
			Status: subagent.Status, Summary: subagent.Summary, Prompt: subagent.Prompt,
			Model: subagent.Model, ReasoningEffort: subagent.ReasoningEffort,
			UpdatedAt: subagent.UpdatedAt,
		})
	}
	httpapi.WriteJSON(w, http.StatusOK, response)
}
