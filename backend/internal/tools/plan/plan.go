package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/tools"
)

const ToolName = "update_plan"

type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventPublisher interface {
	Publish(event sessionevents.Event)
}

type Tool struct {
	Store  SessionEventAppender
	Events SessionEventPublisher
}

type input struct {
	Explanation *string     `json:"explanation,omitempty"`
	Plan        *[]planItem `json:"plan"`
}

type planItem struct {
	Step   *string     `json:"step"`
	Status *stepStatus `json:"status"`
}

type stepStatus string

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		ToolName,
		"Updates the task plan.\nProvide an optional explanation and a list of plan items, each with a step and status.\nAt most one step can be in_progress at a time.\n",
		false,
		tools.ObjectSchema(map[string]any{
			"explanation": map[string]any{"type": "string"},
			"plan": map[string]any{
				"type":        "array",
				"description": "The list of steps",
				"items": tools.ObjectSchema(map[string]any{
					"step": map[string]any{"type": "string"},
					"status": map[string]any{
						"type":        "string",
						"description": "One of: pending, in_progress, completed",
					},
				}, []string{"step", "status"}),
			},
		}, []string{"plan"}),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Store == nil {
		return tools.Result{}, errors.New("update_plan store is nil")
	}
	sessionID := sessioncontext.SessionID(ctx)
	if strings.TrimSpace(sessionID) == "" {
		return tools.Result{}, errors.New("update_plan requires a session context")
	}
	req, err := parseInputs(inputs)
	if err != nil {
		return tools.Result{}, err
	}
	entries, err := planEntries(*req.Plan)
	if err != nil {
		return tools.Result{}, err
	}

	eventType := "plan_update"
	awaitingApproval := false
	result := "Plan updated"
	if sessioncontext.CollaborationMode(ctx) == sessioncontext.CollaborationModePlan {
		eventType = "proposed_plan"
		awaitingApproval = true
		result = "Plan proposed"
	}
	event := sessionevents.Event{
		SessionID: sessionID,
		Type:      eventType,
		Plan: &sessionevents.PlanEvent{
			Explanation:      optionalString(req.Explanation),
			Plan:             entries,
			AwaitingApproval: awaitingApproval,
		},
	}
	events := []sessionevents.Event{event}
	if err := t.Store.AppendSessionEvents(sessionID, events...); err != nil {
		return tools.Result{}, err
	}
	if t.Events != nil {
		t.Events.Publish(events[0])
	}

	return tools.Result{Content: result}, nil
}

func (s *stepStatus) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	switch value {
	case "pending", "in_progress", "completed":
		*s = stepStatus(value)
		return nil
	default:
		return fmt.Errorf("invalid status %q, expected pending, in_progress, or completed", value)
	}
}

func parseInputs(inputs map[string]any) (input, error) {
	data, err := json.Marshal(inputs)
	if err != nil {
		return input{}, fmt.Errorf("failed to parse function arguments: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var req input
	if err := dec.Decode(&req); err != nil {
		return input{}, fmt.Errorf("failed to parse function arguments: %w", err)
	}
	if req.Plan == nil {
		return input{}, errors.New("failed to parse function arguments: missing field `plan`")
	}
	for i, item := range *req.Plan {
		if item.Step == nil {
			return input{}, fmt.Errorf("failed to parse function arguments: missing field `step` at plan[%d]", i)
		}
		if item.Status == nil {
			return input{}, fmt.Errorf("failed to parse function arguments: missing field `status` at plan[%d]", i)
		}
	}
	return req, nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func planEntries(items []planItem) ([]sessionevents.PlanEntry, error) {
	entries := make([]sessionevents.PlanEntry, 0, len(items))
	inProgress := 0
	for i, item := range items {
		if item.Step == nil {
			return nil, fmt.Errorf("plan[%d].step is required", i)
		}
		if item.Status == nil {
			return nil, fmt.Errorf("plan[%d].status is required", i)
		}
		if *item.Status == "in_progress" {
			inProgress++
			if inProgress > 1 {
				return nil, errors.New("at most one plan step can be in_progress")
			}
		}
		content, ok := sessionevents.NormalizeProgressEntryContent(*item.Step)
		if !ok {
			return nil, fmt.Errorf("plan[%d].step must be a short plain-text task", i)
		}
		entries = append(entries, sessionevents.PlanEntry{
			Content: content,
			Status:  string(*item.Status),
		})
	}
	return entries, nil
}
