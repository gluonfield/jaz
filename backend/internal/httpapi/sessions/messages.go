package sessions

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/transcript"
)

const (
	initialTurns = 14
	maxTurns     = 48
)

type MessagesHandler struct {
	service *transcript.Service
}

type messagesResponse struct {
	*acpStateResponse
	Session          sessionview.Response           `json:"session"`
	Messages         []storage.Message              `json:"messages"`
	Events           []sessionview.EventResponse    `json:"events"`
	HasEarlier       bool                           `json:"has_earlier"`
	BeforeMessageSeq int64                          `json:"before_message_seq,omitempty"`
	BeforeEventSeq   int64                          `json:"before_event_seq,omitempty"`
	HistoryRevision  int64                          `json:"history_revision"`
	LatestEventSeq   int64                          `json:"latest_event_seq"`
	ACPMeta          map[string]transcript.Metadata `json:"acp_meta,omitempty"`
	ACPChildren      []acp.Job                      `json:"acp_children,omitempty"`
	ChildPermissions []sessionevents.ACPPermission  `json:"acp_child_permissions,omitempty"`
}

type acpStateResponse struct {
	ACPState           string                        `json:"acp_state"`
	ACPAssistant       string                        `json:"acp_assistant"`
	ACPThought         string                        `json:"acp_thought"`
	ACPModes           acp.ModeState                 `json:"acp_modes"`
	ACPPlan            []sessionevents.PlanEntry     `json:"acp_plan"`
	ACPToolCalls       []sessionevents.ACPToolCall   `json:"acp_tool_calls"`
	ACPPermissions     []sessionevents.ACPPermission `json:"acp_permissions"`
	ACPError           string                        `json:"acp_error"`
	ACPGoalRequested   bool                          `json:"acp_goal_requested"`
	ACPActiveOperation string                        `json:"acp_active_operation"`
	ACPLastEventAt     time.Time                     `json:"acp_last_event_at"`
	ACPLastToolAt      time.Time                     `json:"acp_last_tool_at"`
}

func NewMessagesHandler(service *transcript.Service) *MessagesHandler {
	return &MessagesHandler{service: service}
}

func (h *MessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, err := pageRequest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	view, err := h.service.Load(r.Context(), r.PathValue("session"), request)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrSessionNotFound):
			httpapi.WriteError(w, http.StatusNotFound, err)
		case errors.Is(err, storage.ErrTranscriptChanged):
			httpapi.WriteError(w, http.StatusConflict, err)
		default:
			httpapi.WriteError(w, http.StatusInternalServerError, err)
		}
		return
	}
	page := view.Page
	events := page.Events
	children := view.Children
	if sessioncontext.ClientPlatform(r.Context()) == "mobile" {
		events = mobileEvents(events)
		children = mobileJobs(children)
		if view.Snapshot != nil {
			snapshot := mobileJob(*view.Snapshot)
			view.Snapshot = &snapshot
		}
	}
	response := messagesResponse{
		Session: sessionview.Public(view.Session), Messages: sessionview.Messages(page.Messages),
		Events: sessionview.Events(events), HasEarlier: page.HasEarlier,
		BeforeMessageSeq: page.BeforeMessageSeq, BeforeEventSeq: page.BeforeEventSeq,
		HistoryRevision: page.HistoryRevision, LatestEventSeq: page.LatestEventSeq,
		ACPMeta: view.Meta, ACPChildren: children, ChildPermissions: view.ChildPermissions,
	}
	if view.Snapshot != nil {
		response.acpStateResponse = stateResponse(*view.Snapshot)
	}
	httpapi.WriteJSON(w, http.StatusOK, response)
}

func stateResponse(job acp.Job) *acpStateResponse {
	return &acpStateResponse{
		ACPState: job.State, ACPAssistant: job.Assistant, ACPThought: job.Thought,
		ACPModes: job.Modes, ACPPlan: job.Plan, ACPToolCalls: job.ToolCalls,
		ACPPermissions: job.Permissions, ACPError: job.Error, ACPGoalRequested: job.GoalRequested,
		ACPActiveOperation: job.ActiveOperation, ACPLastEventAt: job.LastEventAt, ACPLastToolAt: job.LastToolAt,
	}
}

func pageRequest(r *http.Request) (storage.TranscriptPageRequest, error) {
	query := r.URL.Query()
	beforeMessageSeq, err := nonNegative(query.Get("before_message_seq"), "before_message_seq")
	if err != nil {
		return storage.TranscriptPageRequest{}, err
	}
	beforeEventSeq, err := nonNegative(query.Get("before_event_seq"), "before_event_seq")
	if err != nil {
		return storage.TranscriptPageRequest{}, err
	}
	turns := initialTurns
	if raw := strings.TrimSpace(query.Get("turns")); raw != "" {
		turns, err = strconv.Atoi(raw)
		if err != nil || turns <= 0 || turns > maxTurns {
			return storage.TranscriptPageRequest{}, fmt.Errorf("turns must be between 1 and %d", maxTurns)
		}
	}
	revision, err := nonNegative(query.Get("history_revision"), "history_revision")
	if err != nil {
		return storage.TranscriptPageRequest{}, err
	}
	if (beforeMessageSeq > 0 || beforeEventSeq > 0) && revision == 0 {
		return storage.TranscriptPageRequest{}, fmt.Errorf("history_revision is required with a history cursor")
	}
	return storage.TranscriptPageRequest{
		BeforeMessageSeq: beforeMessageSeq, BeforeEventSeq: beforeEventSeq,
		HistoryRevision: revision, Turns: turns,
	}, nil
}

func nonNegative(raw, name string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}
