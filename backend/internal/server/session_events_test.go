package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/wins/jaz/backend/internal/voice/live"
)

func TestStreamSessionEventsKeepsVoiceDiagnosticsPrivate(t *testing.T) {
	for _, replay := range []bool{true, false} {
		t.Run(fmt.Sprint("replay=", replay), func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.CreateSession(storage.CreateSession{Slug: "voice"})
			if err != nil {
				t.Fatal(err)
			}
			bus := sessionevents.New()
			voice := live.NewTranscript(store, bus)
			publish := func() {
				t.Helper()
				if err := voice.Record(session.ID, []live.Diagnostic{{ID: "receipt", CallID: "call", Stage: "append_ack", At: time.Now().UTC()}}); err != nil {
					t.Fatal(err)
				}
				if err := voice.Append(session.ID, []sessionevents.VoiceMessage{{ID: "speech", CallID: "call", Role: "assistant", Text: "Here are the slides."}}); err != nil {
					t.Fatal(err)
				}
			}
			if replay {
				publish()
			}
			srv := &Server{Store: store, Events: bus}
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				srv.streamSessionEvents(w, r, session.ID)
			}))
			t.Cleanup(httpServer.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if !replay {
				publish()
			}
			scanner := bufio.NewScanner(res.Body)
			for scanner.Scan() {
				data, ok := strings.CutPrefix(scanner.Text(), "data: ")
				if !ok {
					continue
				}
				var event sessionevents.Event
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					t.Fatal(err)
				}
				if event.Voice == nil || event.Voice.Text != "Here are the slides." {
					t.Fatalf("unexpected voice stream event: %s", data)
				}
				return
			}
			t.Fatalf("speech missing from stream: %v", scanner.Err())
		})
	}
}

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
