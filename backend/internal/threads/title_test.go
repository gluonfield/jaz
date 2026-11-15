package threads

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSetThreadTitleUsesCallingThreadAndPublishesChanges(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	caller, err := store.CreateSession(storage.CreateSession{Title: "Generated"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateSession(storage.CreateSession{Title: "Other thread"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpdateGeneratedSessionTitle(caller.ID, "Locked generated title"); err != nil {
		t.Fatal(err)
	}
	events := sessionevents.New()
	sub := events.Subscribe(t.Context(), caller.ID)
	tools := TitleTools{Store: store, Events: events}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	tools.AddTo(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(mcpsession.HeaderName, caller.ID)
		handler.ServeHTTP(w, r)
	}))
	defer endpoint.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	connection, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: endpoint.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	want := strings.Repeat("題", 100)
	rejected, err := connection.CallTool(t.Context(), &mcp.CallToolParams{Name: "set_thread_title", Arguments: map[string]any{"title": "Cross-thread attempt", "threadId": other.ID}})
	if err != nil || !rejected.IsError {
		t.Fatalf("accepted a target-thread argument: %+v %v", rejected, err)
	}
	result, err := connection.CallTool(t.Context(), &mcp.CallToolParams{Name: "set_thread_title", Arguments: map[string]any{"title": "  " + want + " "}})
	if err != nil || result.IsError {
		t.Fatalf("title call failed: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	var out TitleResult
	if err != nil || json.Unmarshal(encoded, &out) != nil || out.Title != want || out.ManualTitlePreserved {
		t.Fatalf("title result = %s %v", encoded, err)
	}
	current, err := store.LoadSession(caller.ID)
	if err != nil || current.Title != want || !current.TitleLocked || current.ManualTitle {
		t.Fatalf("calling thread not updated: %+v %v", current, err)
	}
	untouched, err := store.LoadSession(other.ID)
	if err != nil || untouched.Title != "Other thread" {
		t.Fatalf("another thread changed: %+v %v", untouched, err)
	}
	select {
	case event := <-sub:
		if event.Type != sessionevents.TypeSession || event.SessionID != caller.ID {
			t.Fatalf("wrong session event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("title change did not publish a session event")
	}
	if err := store.UpdateSessionTitle(caller.ID, "User chosen"); err != nil {
		t.Fatal(err)
	}
	result, err = connection.CallTool(t.Context(), &mcp.CallToolParams{Name: "set_thread_title", Arguments: map[string]any{"title": "Agent replacement"}})
	if err != nil || result.IsError {
		t.Fatalf("manual title call failed: %+v %v", result, err)
	}
	encoded, marshalErr := json.Marshal(result.StructuredContent)
	if marshalErr != nil || json.Unmarshal(encoded, &out) != nil || out.Title != "User chosen" || !out.ManualTitlePreserved {
		t.Fatalf("manual preservation not reported: %s %v", encoded, err)
	}
	for _, input := range []TitleInput{{}, {Title: "  "}} {
		if _, _, err := tools.Set(t.Context(), &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(caller.ID)}}, input); err == nil {
			t.Fatal("accepted an empty title")
		}
	}
	if _, _, err := tools.Set(t.Context(), nil, TitleInput{Title: "No caller"}); err == nil {
		t.Fatal("accepted a call without a thread")
	}
}
