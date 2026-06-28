package acp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestCreateElicitationPublishesQuestionsAndReturnsAnswers(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "claude-question", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Cwd: t.TempDir()}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	result := make(chan json.RawMessage, 1)
	errs := make(chan *jsonrpc.Error, 1)

	go func() {
		raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
			Method: acpschema.ClientMethodElicitationCreate,
			Params: mustJSON(t, map[string]any{
				"mode":       "form",
				"sessionId":  "acp-session",
				"toolCallId": "ask-1",
				"message":    "Which macros page should I build?",
				"requestedSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question_0": map[string]any{
							"type":  "string",
							"title": "Macros type",
							"oneOf": []map[string]any{
								{
									"const": "Nutrition",
									"title": "Nutrition - Protein, carbs, and fat",
									"_meta": map[string]any{
										"_claude/askUserQuestionOption": map[string]any{
											"description": "Protein, carbs, and fat",
										},
									},
								},
							},
						},
						"question_0_custom": map[string]any{"type": "string", "title": "Other"},
					},
				},
			}),
		})
		if rpcErr != nil {
			errs <- rpcErr
			return
		}
		result <- raw
	}()

	var requestID string
	select {
	case event := <-sub:
		if event.Type != "permission_request" || event.Permission == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		requestID = event.Permission.ID
		if event.Permission.ToolCallID != "ask-1" ||
			len(event.Permission.Questions) != 1 ||
			event.Permission.Questions[0].ID != "question_0" ||
			event.Permission.Questions[0].Question != "Which macros page should I build?" ||
			event.Permission.Questions[0].Options[0].Description != "Protein, carbs, and fat" {
			t.Fatalf("permission = %#v", event.Permission)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   session.ID,
		RequestID: requestID,
		Answers: map[string]InteractiveAnswerValue{
			"question_0": {Answers: []string{"Nutrition"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-result:
		var got acpschema.CreateElicitationResponse
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.Action != "accept" || string(got.Content["question_0"]) != `"Nutrition"` {
			t.Fatalf("elicitation response = %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestCreateElicitationPlainTextAnswerUsesRequestedField(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "plain-question", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	manager := NewManager(store, Config{}, nil)
	manager.Events = events
	manager.jobsByID[session.ID] = &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Cwd: t.TempDir()}}
	manager.jobsByACP["acp-session"] = manager.jobsByID[session.ID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := events.Subscribe(ctx, session.ID)
	result := make(chan json.RawMessage, 1)
	errs := make(chan *jsonrpc.Error, 1)

	go func() {
		raw, rpcErr := manager.handleJSONRPC(ctx, jsonrpc.Request{
			Method: acpschema.ClientMethodElicitationCreate,
			Params: mustJSON(t, map[string]any{
				"mode":      "form",
				"sessionId": "acp-session",
				"message":   "What name should I use?",
				"requestedSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "title": "Name"},
					},
				},
			}),
		})
		if rpcErr != nil {
			errs <- rpcErr
			return
		}
		result <- raw
	}()

	var requestID string
	select {
	case event := <-sub:
		if event.Type != "permission_request" || event.Permission == nil {
			t.Fatalf("unexpected event %#v", event)
		}
		requestID = event.Permission.ID
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   session.ID,
		RequestID: requestID,
		Answers: map[string]InteractiveAnswerValue{
			"name": {Answers: []string{"Ada"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-result:
		var got acpschema.CreateElicitationResponse
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if string(got.Content["name"]) != `"Ada"` {
			t.Fatalf("elicitation response = %s", raw)
		}
		if _, ok := got.Content["name_custom"]; ok {
			t.Fatalf("unexpected custom field in response: %s", raw)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
