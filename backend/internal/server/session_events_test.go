package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type countingEventStore struct {
	storage.Store
	fullLoads  int
	afterLoads int
}

func (s *countingEventStore) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	s.fullLoads++
	return s.Store.LoadSessionEvents(id)
}

func (s *countingEventStore) LoadSessionEventsAfter(id string, afterSeq int64) ([]sessionevents.Event, error) {
	s.afterLoads++
	return s.Store.LoadSessionEventsAfter(id, afterSeq)
}

func TestStreamSessionEventsReplaysPersistedEventsAfterSeq(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "one"},
		sessionevents.Event{Type: "acp_message", Content: "two"},
		sessionevents.Event{Type: "acp_message", Content: "three"},
	); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "", "")
	body := res.Body.String()

	if strings.Contains(body, "one") {
		t.Fatalf("replayed event before after_seq: %s", body)
	}
	if !strings.Contains(body, "id: 2\n") || !strings.Contains(body, "two") {
		t.Fatalf("missing replayed seq 2 event: %s", body)
	}
	if !strings.Contains(body, "id: 3\n") || !strings.Contains(body, "three") {
		t.Fatalf("missing replayed seq 3 event: %s", body)
	}
	if strings.HasPrefix(body, "event:") || strings.Contains(body, "\nevent:") {
		t.Fatalf("session event stream should use unnamed SSE frames: %s", body)
	}
	if got := res.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}
}

func TestStreamSessionEventsReplaysProjectedTextAsReplacement(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "projected-events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type:    sessionevents.TypeACPMessage,
			Content: content,
			ACP: &sessionevents.ACPEvent{
				ID: session.ID, Agent: "codex", SessionID: "acp-session", State: acp.StateRunning, TextRunID: "message:one",
			},
		}
	}
	if err := store.AppendSessionEvents(session.ID, text("Hel"), text("lo")); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "", "")
	body := res.Body.String()
	if !strings.Contains(body, `"content":"Hello"`) || !strings.Contains(body, `"projection_op":"replace"`) {
		t.Fatalf("projected replay = %s", body)
	}
	if !strings.Contains(body, `"projection_key":"acp_text:`) {
		t.Fatalf("projected replay has no backend identity: %s", body)
	}
}

func TestStreamSessionEventsUsesBoundedReplayForProjectedEvents(t *testing.T) {
	base, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := base.CreateSession(storage.CreateSession{Slug: "bounded-events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content, op string) sessionevents.Event {
		return sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: content,
			ProjectionKey: "acp_text:thread:agent:message:one", ProjectionOp: op,
			ACP: &sessionevents.ACPEvent{
				ID: session.ID, Agent: "codex", SessionID: "acp-session", State: acp.StateRunning, TextRunID: "message:one",
			},
		}
	}
	if err := base.AppendSessionEvents(session.ID, text("Hel", ""), text("lo", sessionevents.ProjectionAppend)); err != nil {
		t.Fatal(err)
	}
	store := &countingEventStore{Store: base}
	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "", "")
	body := res.Body.String()
	if store.afterLoads != 1 || store.fullLoads != 0 {
		t.Fatalf("event loads = after %d, full %d", store.afterLoads, store.fullLoads)
	}
	if !strings.Contains(body, `"content":"lo"`) || !strings.Contains(body, `"projection_op":"append"`) {
		t.Fatalf("bounded replay = %s", body)
	}
}

func TestStreamSessionEventsDoesNotReloadInitialLegacyHistory(t *testing.T) {
	base, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := base.CreateSession(storage.CreateSession{Slug: "legacy-events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: content,
			ACP: &sessionevents.ACPEvent{
				ID: session.ID, State: acp.StateRunning, TextRunID: "message:one",
			},
		}
	}
	if err := base.AppendSessionEvents(session.ID, text("Hel"), text("lo")); err != nil {
		t.Fatal(err)
	}
	store := &countingEventStore{Store: base}
	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events", "", "")

	if store.afterLoads != 1 || store.fullLoads != 0 {
		t.Fatalf("event loads = after %d, full %d", store.afterLoads, store.fullLoads)
	}
	if !strings.Contains(res.Body.String(), `"content":"Hello"`) {
		t.Fatalf("projected replay = %s", res.Body.String())
	}
}

func TestStreamSessionEventsResumesFromLastEventID(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "one"},
		sessionevents.Event{Type: "acp_message", Content: "two"},
		sessionevents.Event{Type: "acp_message", Content: "three"},
	); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "2", "")
	body := res.Body.String()

	if strings.Contains(body, "one") || strings.Contains(body, "two") {
		t.Fatalf("Last-Event-ID was not respected: %s", body)
	}
	if !strings.Contains(body, "id: 3\n") || !strings.Contains(body, "three") {
		t.Fatalf("missing replayed seq 3 event: %s", body)
	}
}

func TestStreamSessionEventsMobileProjectsToolPayload(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "mobile-events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	heavyCall := sessionevents.ACPToolCall{
		ID:       "tool-1",
		Title:    "rg release",
		Status:   "completed",
		Kind:     "terminal",
		ToolName: "shell",
		Content: []sessionevents.ACPToolContent{{
			Type: "text",
			Text: "very large replayed tool result",
		}},
		RawInput: map[string]any{
			"cmd": "expensive replayed command input",
		},
		Runtime: sessionevents.ACPToolRuntime{ElapsedTimeSeconds: 12.5},
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: "acp_tool",
		ACP: &sessionevents.ACPEvent{
			ID:        session.ID,
			Slug:      session.Slug,
			Agent:     "codex",
			SessionID: "acp-session",
			State:     acp.StateIdle,
			ToolCalls: []sessionevents.ACPToolCall{heavyCall},
		},
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events", "", "mobile")
	body := res.Body.String()

	for _, forbidden := range []string{
		"very large replayed tool result",
		"expensive replayed command input",
		`"kind":"terminal"`,
		`"tool_name":"shell"`,
		`"elapsed_time_seconds"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("mobile SSE contains stripped payload %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"id":"tool-1"`) || !strings.Contains(body, `"title":"rg release"`) {
		t.Fatalf("mobile SSE missing tool summary: %s", body)
	}
}

func TestStreamSessionEventsClearsRequestedGoalSnapshots(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal-events", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeGoalUpdate,
		Goal: &sessionevents.GoalEvent{
			Identity: goal.Identity{
				Objective: "raw user prompt text",
				Status:    goal.StatusRequested,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, Events: sessionevents.New()}
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events", "", "")
	body := res.Body.String()

	if strings.Contains(body, "raw user prompt text") || strings.Contains(body, `"goal":`) {
		t.Fatalf("requested goal leaked into SSE: %s", body)
	}
	if !strings.Contains(body, `"type":"goal_clear"`) {
		t.Fatalf("requested goal was not streamed as clear: %s", body)
	}
}

func streamSessionEventsForTest(t *testing.T, srv *Server, sessionID, target, lastEventID, clientPlatform string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	if clientPlatform != "" {
		req.Header.Set("X-Jaz-Client-Platform", clientPlatform)
	}
	res := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.streamSessionEvents(res, req, sessionID)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close after context cancellation")
	}
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	return res
}
