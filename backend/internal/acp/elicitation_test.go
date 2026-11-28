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
									"const":       "Nutrition",
									"title":       "Nutrition",
									"description": "Protein, carbs, and fat",
								},
							},
						},
						"question_0_custom": map[string]any{"type": "string", "title": "Other", "_meta": map[string]any{
							"jetbrains": map[string]any{"air": map[string]any{"version": 1, "customAnswer": map[string]any{"questionId": "question_0", "isCustomAnswer": true}}},
						}},
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

func TestCreateElicitationCancelsWhenPromptSuccessorQueued(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queued-question", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: session.ID, ACPSession: "acp-session", Cwd: t.TempDir()}, steerMethod: steerPromptQueueing}
	job.startTurn(CompletionInline, false, false)
	job.turn.promptCalls++
	manager.jobsByID[session.ID] = job
	manager.jobsByACP["acp-session"] = job

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodElicitationCreate,
		Params: mustJSON(t, map[string]any{
			"mode":      "form",
			"sessionId": "acp-session",
			"message":   "Which workspace?",
			"requestedSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"choice": map[string]any{"type": "string", "title": "Choice"},
				},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	var got acpschema.CreateElicitationResponse
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Action != "cancel" {
		t.Fatalf("action = %q", got.Action)
	}
	if len(manager.pendingPermission) != 0 {
		t.Fatalf("pending permission registered: %#v", manager.pendingPermission)
	}
	job.mu.RLock()
	permissions := len(job.Permissions)
	job.mu.RUnlock()
	if permissions != 0 {
		t.Fatalf("job permissions = %d", permissions)
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

func TestElicitationArraySupportsMultipleSelection(t *testing.T) {
	var schema acpschema.ElicitationSchema
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{"workloads":{"type":"array","items":{"type":"string","enum":["SQL","Spark"]}}}}`), &schema); err != nil {
		t.Fatal(err)
	}
	questions, fields := elicitationQuestions("Which workloads?", &schema)
	if len(questions) != 1 || !questions[0].MultiSelect || len(questions[0].Options) != 2 {
		t.Fatalf("array question = %#v", questions)
	}
	raw, err := encodeElicitationResponse(fields, map[string]InteractiveAnswerValue{
		"workloads": {Answers: []string{"SQL", "Spark"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got acpschema.CreateElicitationResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Content["workloads"]) != `["SQL","Spark"]` {
		t.Fatalf("array response = %s", raw)
	}
}

func TestElicitationArraySeparatesCustomAnswer(t *testing.T) {
	var schema acpschema.ElicitationSchema
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{
		"workloads":{"type":"array","items":{"type":"string","enum":["SQL","Spark"]}},
		"workloads_custom":{"type":"string","_meta":{"jetbrains":{"air":{"customAnswer":{"questionId":"workloads"}}}}}
	}}`), &schema); err != nil {
		t.Fatal(err)
	}
	questions, fields := elicitationQuestions("Which workloads?", &schema)
	if len(questions) != 1 || !questions[0].MultiSelect || !questions[0].IsOther {
		t.Fatalf("array question = %#v", questions)
	}
	for _, test := range []struct {
		name    string
		answers []string
		choices string
		custom  string
	}{
		{name: "choices", answers: []string{"SQL", "Spark"}, choices: `["SQL","Spark"]`},
		{name: "mixed", answers: []string{"SQL", "Custom ingestion"}, choices: `["SQL"]`, custom: `"Custom ingestion"`},
		{name: "custom", answers: []string{"Custom ingestion"}, custom: `"Custom ingestion"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := encodeElicitationResponse(fields, map[string]InteractiveAnswerValue{
				"workloads": {Answers: test.answers},
			})
			if err != nil {
				t.Fatal(err)
			}
			var got acpschema.CreateElicitationResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatal(err)
			}
			if string(got.Content["workloads"]) != test.choices || string(got.Content["workloads_custom"]) != test.custom {
				t.Fatalf("elicitation response = %s", raw)
			}
		})
	}
}

func TestElicitationQuestionsReadCodexUserInputForm(t *testing.T) {
	const message = "Codex needs your input to continue."
	var schema acpschema.ElicitationSchema
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{
		"migration_strategy":{"type":"string","title":"How should existing rows be migrated?","description":"Migration",
			"_meta":{"codex":{"isOther":true,"isSecret":false}},
			"oneOf":[{"const":"Lazy backfill","title":"Lazy backfill","description":"Migrate rows on first read."},
				{"const":"None of the above","title":"None of the above","description":"Provide a different answer in the note field."}]},
		"migration_strategy_note":{"type":"string","title":"Additional answer or note",
			"_meta":{"codex":{"questionId":"migration_strategy","role":"user_note","isSecret":false}}},
		"ticket":{"type":"string","title":"Which issue should the commit reference?","description":"Ticket",
			"_meta":{"codex":{"isOther":false,"isSecret":true}}}},
		"required":["migration_strategy","ticket"]}`), &schema); err != nil {
		t.Fatal(err)
	}

	questions, fields := elicitationQuestions(message, &schema)
	if len(questions) != 2 {
		t.Fatalf("questions = %#v", questions)
	}
	migration := questions[0]
	if migration.ID != "migration_strategy" || migration.Question != "How should existing rows be migrated?" ||
		migration.Header != "Migration" || !migration.IsOther || len(migration.Options) != 2 ||
		migration.Options[0].Description != "Migrate rows on first read." {
		t.Fatalf("migration question = %#v", migration)
	}
	if questions[1].Question != "Which issue should the commit reference?" || questions[1].Header != "Ticket" || !questions[1].IsSecret || migration.IsSecret {
		t.Fatalf("ticket question = %#v", questions[1])
	}

	single := acpschema.ElicitationSchema{Properties: map[string]acpschema.ElicitationPropertySchema{"ticket": schema.Properties["ticket"]}}
	if got, _ := elicitationQuestions(message, &single); got[0].Question != "Which issue should the commit reference?" {
		t.Fatalf("single codex question = %q", got[0].Question)
	}

	raw, err := encodeElicitationResponse(fields, map[string]InteractiveAnswerValue{
		"migration_strategy": {Answers: []string{"Dual-write both columns"}},
		"ticket":             {Answers: []string{"JAZ-12"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got acpschema.CreateElicitationResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Content) != 2 || string(got.Content["migration_strategy_note"]) != `"Dual-write both columns"` ||
		string(got.Content["ticket"]) != `"JAZ-12"` {
		t.Fatalf("elicitation response = %s", raw)
	}
}

func TestElicitationQuestionsPairCompanionOtherFields(t *testing.T) {
	const message = "Please answer the following questions."
	var schema acpschema.ElicitationSchema
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{
		"q1":{"type":"string","title":"Where did the bug happen?",
			"oneOf":[{"const":"1","title":"1 — Browser"},{"const":"2","title":"2 — Desktop"}]},
		"q1_other":{"type":"string","title":"Other","description":"Optional free-form answer to: Where did the bug happen?"},
		"q2":{"type":"array","title":"Which conditions applied?",
			"items":{"type":"string","anyOf":[{"const":"1","title":"1 — Normal chat"},{"const":"2","title":"2 — Side panel"}]}},
		"q2_other":{"type":"string","title":"Other","description":"Optional free-form answer to: Which conditions applied?"}
	}}`), &schema); err != nil {
		t.Fatal(err)
	}

	questions, fields := elicitationQuestions(message, &schema)
	if len(questions) != 2 {
		t.Fatalf("questions = %#v", questions)
	}
	if questions[0].ID != "q1" || questions[0].Question != "Where did the bug happen?" || !questions[0].IsOther || questions[0].MultiSelect || len(questions[0].Options) != 2 {
		t.Fatalf("q1 = %#v", questions[0])
	}
	if questions[1].ID != "q2" || questions[1].Question != "Which conditions applied?" || !questions[1].IsOther || !questions[1].MultiSelect || len(questions[1].Options) != 2 {
		t.Fatalf("q2 = %#v", questions[1])
	}

	// Single-question form with companion _other must keep the question title rather than the generic banner message.
	single := acpschema.ElicitationSchema{Properties: map[string]acpschema.ElicitationPropertySchema{
		"q1":       schema.Properties["q1"],
		"q1_other": schema.Properties["q1_other"],
	}}
	singleQuestions, _ := elicitationQuestions(message, &single)
	if len(singleQuestions) != 1 || singleQuestions[0].Question != "Where did the bug happen?" || !singleQuestions[0].IsOther {
		t.Fatalf("single question = %#v", singleQuestions)
	}

	// Standalone _other field without a choice parent stays a normal question.
	standalone := acpschema.ElicitationSchema{Properties: map[string]acpschema.ElicitationPropertySchema{
		"notes_other": {Type: "string", Title: "Extra notes"},
	}}
	if got, _ := elicitationQuestions("", &standalone); len(got) != 1 || got[0].ID != "notes_other" || !got[0].IsOther {
		t.Fatalf("standalone _other = %#v", got)
	}

	raw, err := encodeElicitationResponse(fields, map[string]InteractiveAnswerValue{
		"q1": {Answers: []string{"Custom environment"}},
		"q2": {Answers: []string{"1", "With mention"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got acpschema.CreateElicitationResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Content["q1_other"]) != `"Custom environment"` ||
		string(got.Content["q1"]) != "" ||
		string(got.Content["q2"]) != `["1"]` ||
		string(got.Content["q2_other"]) != `"With mention"` {
		t.Fatalf("elicitation response = %s", raw)
	}
}
