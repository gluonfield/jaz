package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSessionTranscriptFiltersToolsAndRoles(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "coding", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	longResult := strings.Repeat("file contents ", 200)
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "system", Content: "You are jaz."},
		storage.Message{Role: "user", Content: "Fix the bug in parser.go"},
		storage.Message{Role: "assistant", Content: "", Blocks: []storage.Block{
			{Type: storage.BlockTypeReasoning, Text: "thinking..."},
			{Type: storage.BlockTypeTool, ID: "call-1", Name: "exec", InputJSON: `{"command":"cat parser.go"}`, Result: longResult},
		}},
		storage.Message{Role: "assistant", Content: "Fixed: the parser dropped trailing newlines. User prefers tabs over spaces."},
	); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/transcript", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("transcript status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Messages   []TranscriptMessage `json:"messages"`
		ToolCounts map[string]int      `json:"tool_counts"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("expected user + final assistant only, got %#v", got.Messages)
	}
	if got.Messages[0].Role != "user" || got.Messages[1].Role != "assistant" || len(got.Messages[1].Tools) != 0 {
		t.Fatalf("unexpected transcript %#v", got.Messages)
	}
	if got.ToolCounts["exec"] != 1 {
		t.Fatalf("expected tool counts even without tool detail, got %#v", got.ToolCounts)
	}
	if strings.Contains(res.Body.String(), "file contents") || strings.Contains(res.Body.String(), "thinking") {
		t.Fatal("transcript leaked tool result or reasoning")
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/transcript?max_tool_chars=80", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("transcript status = %d, body = %s", res.Code, res.Body.String())
	}
	got.Messages = nil
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("expected tool-bearing assistant message included, got %#v", got.Messages)
	}
	tools := got.Messages[1].Tools
	if len(tools) != 1 || tools[0].Name != "exec" || !strings.HasSuffix(tools[0].Detail, "...") || len(tools[0].Detail) > 90 {
		t.Fatalf("unexpected compressed tools %#v", tools)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/transcript?max_tool_chars=-1", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for negative max_tool_chars, got %d", res.Code)
	}
}

func TestListSessionsUpdatedSince(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.CreateSession(storage.CreateSession{Slug: "old", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	cutoff := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	fresh, err := store.CreateSession(storage.CreateSession{Slug: "fresh", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions?updated_since="+cutoff.Format(time.RFC3339Nano), nil))
	if res.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", res.Code, res.Body.String())
	}
	var listed struct {
		Sessions []storage.Session `json:"sessions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != fresh.ID {
		t.Fatalf("expected only fresh session %s, got %#v (old=%s)", fresh.ID, listed.Sessions, old.ID)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions?updated_since=not-a-time", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for malformed updated_since, got %d", res.Code)
	}
}
