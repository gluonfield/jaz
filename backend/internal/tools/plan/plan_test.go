package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/tools"
)

type fakeAppender struct {
	events []sessionevents.Event
}

func (f *fakeAppender) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	for i := range events {
		if events[i].SessionID == "" {
			events[i].SessionID = id
		}
	}
	f.events = append(f.events, events...)
	return nil
}

type fakePublisher struct {
	events []sessionevents.Event
}

func (f *fakePublisher) Publish(event sessionevents.Event) {
	f.events = append(f.events, event)
}

func TestDefinitionMatchesCodexShape(t *testing.T) {
	def := (&Tool{}).Definition()
	fn := def.GetFunction()
	if fn == nil || fn.Name != ToolName {
		t.Fatalf("definition = %#v", def)
	}
	if got := fn.Description.Value; got != "Updates the task plan.\nProvide an optional explanation and a list of plan items, each with a step and status.\nAt most one step can be in_progress at a time.\n" {
		t.Fatalf("description = %q", got)
	}
	if fn.Strict.Value {
		t.Fatalf("strict = true, want false")
	}
	params := map[string]any(fn.Parameters)
	props := params["properties"].(map[string]any)
	status := props["plan"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["status"].(map[string]any)
	if _, ok := status["enum"]; ok {
		t.Fatalf("status schema should not carry enum: %#v", status)
	}
	if status["description"] != "One of: pending, in_progress, completed" {
		t.Fatalf("status schema = %#v", status)
	}
}

func TestUpdatePlanWritesPlanUpdateEvent(t *testing.T) {
	store := &fakeAppender{}
	events := &fakePublisher{}
	ctx := sessioncontext.WithSessionID(context.Background(), "session-1")

	result, err := (&Tool{Store: store, Events: events}).Execute(ctx, map[string]any{
		"explanation": "Plan the work first.",
		"plan": []any{
			map[string]any{"step": "Inspect the code", "status": "completed"},
			map[string]any{"step": "Make the change", "status": "in_progress"},
			map[string]any{"step": "Run tests", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "Plan updated" {
		t.Fatalf("result = %q", result.Content)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %#v", store.events)
	}
	event := store.events[0]
	if event.Type != "plan_update" || event.ACP != nil || event.Plan == nil {
		t.Fatalf("event = %#v", event)
	}
	if event.Plan.Explanation != "Plan the work first." {
		t.Fatalf("explanation = %q", event.Plan.Explanation)
	}
	if len(event.Plan.Plan) != 3 || event.Plan.Plan[1].Status != "in_progress" {
		t.Fatalf("plan = %#v", event.Plan.Plan)
	}
	if len(events.events) != 1 || events.events[0].Type != "plan_update" {
		t.Fatalf("published = %#v", events.events)
	}
}

func TestUpdatePlanAcceptsEmptyPlan(t *testing.T) {
	ctx := sessioncontext.WithSessionID(context.Background(), "session-1")
	store := &fakeAppender{}
	if _, err := (&Tool{Store: store}).Execute(ctx, map[string]any{
		"plan": []any{},
	}); err != nil {
		t.Fatalf("empty plan err = %v", err)
	}
	if len(store.events) != 1 || len(store.events[0].Plan.Plan) != 0 {
		t.Fatalf("empty plan event = %#v", store.events)
	}
}

func TestUpdatePlanRejectsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   string
	}{
		{name: "missing plan", inputs: map[string]any{}, want: "failed to parse function arguments"},
		{name: "unknown field", inputs: map[string]any{"plan": []any{}, "extra": true}, want: "unknown field"},
		{name: "missing step", inputs: map[string]any{"plan": []any{map[string]any{"status": "pending"}}}, want: "missing field `step`"},
		{name: "invalid status", inputs: map[string]any{"plan": []any{map[string]any{"step": "One", "status": "running"}}}, want: "invalid status"},
		{name: "multiple in progress", inputs: map[string]any{"plan": []any{
			map[string]any{"step": "One", "status": "in_progress"},
			map[string]any{"step": "Two", "status": "in_progress"},
		}}, want: "at most one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAppender{}
			_, err := (&Tool{Store: store}).Execute(sessioncontext.WithSessionID(context.Background(), "session-1"), tt.inputs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if len(store.events) != 0 {
				t.Fatalf("events written on parse failure: %#v", store.events)
			}
		})
	}
}

func TestUpdatePlanProposesPlanInPlanMode(t *testing.T) {
	ctx := sessioncontext.WithCollaborationMode(
		sessioncontext.WithSessionID(context.Background(), "session-1"),
		sessioncontext.CollaborationModePlan,
	)
	store := &fakeAppender{}
	result, err := (&Tool{Store: store}).Execute(ctx, map[string]any{
		"explanation": "Ready for approval.",
		"plan": []any{
			map[string]any{"step": "Inspect", "status": "pending"},
			map[string]any{"step": "Patch", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "Plan proposed" {
		t.Fatalf("result = %q", result.Content)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %#v", store.events)
	}
	event := store.events[0]
	if event.Type != "proposed_plan" || event.Plan == nil || !event.Plan.AwaitingApproval {
		t.Fatalf("event = %#v", event)
	}
}

var _ tools.Tool = (*Tool)(nil)
