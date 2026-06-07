package jsonstore

import (
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestSessionsHaveStableUniqueSlugsAndRootListing(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.CreateSession(storage.CreateSession{Slug: "Review ACP Transport", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession(storage.CreateSession{Slug: "review-acp-transport", ParentID: first.ID, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "review-acp-transport" {
		t.Fatalf("unexpected first slug %q", first.Slug)
	}
	if second.Slug != "review-acp-transport-2" {
		t.Fatalf("unexpected second slug %q", second.Slug)
	}

	roots, err := store.ListSessions(storage.SessionFilter{RootOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].ID != first.ID {
		t.Fatalf("unexpected roots %#v", roots)
	}

	resolved, err := store.LoadSession(second.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != second.ID {
		t.Fatalf("resolved %s, want %s", resolved.ID, second.ID)
	}
}

func TestMessagesUseJSONL(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	session, err := store.CreateSession(storage.CreateSession{Slug: "jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("done", nil),
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(store.sessionDir(session.ID), "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.sessionDir(session.ID), "messages.json")); !os.IsNotExist(err) {
		t.Fatalf("messages.json exists or stat failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl lines = %d, want 2", len(lines))
	}
	for _, line := range lines {
		var msg provider.Message
		if err := stdjson.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("invalid jsonl line %q: %v", line, err)
		}
	}
	loaded, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || provider.MessageContent(loaded[1]) != "done" {
		t.Fatalf("loaded messages = %#v", loaded)
	}

	if err := store.AppendMessages(session.ID, provider.UserMessage("again")); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(store.sessionDir(session.ID), "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl lines after append = %d, want 3", len(lines))
	}
}

func TestLoadMessagesFallsBackToLegacyJSON(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-json"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := stdjson.Marshal([]provider.Message{provider.UserMessage("legacy")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.sessionDir(session.ID), "messages.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || provider.MessageContent(loaded[0]) != "legacy" {
		t.Fatalf("loaded legacy messages = %#v", loaded)
	}

	if err := store.AppendMessages(session.ID, provider.AssistantMessage("upgraded", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.sessionDir(session.ID), "messages.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy messages.json exists or stat failed: %v", err)
	}
	loaded, err = store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || provider.MessageContent(loaded[1]) != "upgraded" {
		t.Fatalf("upgraded messages = %#v", loaded)
	}
}
