package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

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
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "")
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
	res := streamSessionEventsForTest(t, srv, session.ID, "/v1/sessions/"+session.ID+"/events?after_seq=1", "2")
	body := res.Body.String()

	if strings.Contains(body, "one") || strings.Contains(body, "two") {
		t.Fatalf("Last-Event-ID was not respected: %s", body)
	}
	if !strings.Contains(body, "id: 3\n") || !strings.Contains(body, "three") {
		t.Fatalf("missing replayed seq 3 event: %s", body)
	}
}

func streamSessionEventsForTest(t *testing.T, srv *Server, sessionID, target, lastEventID string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
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
