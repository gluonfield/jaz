package mobilepush

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/mobilepush"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestStreamSendsAlertsWithoutApplePush(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Title: "Research", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := mobilepush.NewApple(mobilepush.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service := mobilepush.NewService(store, sender, func(string) bool {
		return false
	})
	bus := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := service.Start(ctx, bus, func(err error) {
		t.Errorf("worker: %v", err)
	})
	defer func() {
		cancel()
		<-done
	}()
	server := httptest.NewServer(http.HandlerFunc(Handler{Service: service}.Stream))
	defer server.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", res.Header.Get("Content-Type"))
	}
	bus.Publish(sessionevents.Event{SessionID: session.ID, Type: "permission_request", At: time.Now(), Permission: &sessionevents.ACPPermission{
		ID: "question", SessionID: session.ID, Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Which supplier?"}},
	}})
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var alert map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &alert); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	if alert["session_id"] != session.ID || alert["state"] != "needs_you" || alert["kind"] != "chat" || alert["title"] != "Research" || alert["body"] != "Which supplier?" || alert["request_id"] != "question" || alert["key"] != "question:question" {
		t.Fatalf("streamed alert = %#v", alert)
	}
}
