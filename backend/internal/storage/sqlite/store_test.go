package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionsHaveStableUniqueSlugsAndRootListing(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

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

func TestSaveMessagesRejectsDanglingToolCall(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
	err = store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
	})
	if err == nil {
		t.Fatal("expected dangling tool call to be rejected")
	}
	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 0 {
		t.Fatalf("dangling tool call was mirrored to JSON: %#v", mirrored)
	}
}

func TestToolCallAndResultPersistAsOneAssistantRecord(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	call := provider.FunctionToolCall("call_1", "mock", `{"value":"ok"}`)
	err = store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("", []provider.ToolCall{call}),
		provider.ToolMessage(`{"status":"completed"}`, "call_1"),
		provider.AssistantMessage("done", nil),
	})
	if err != nil {
		t.Fatal(err)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	if len(records[1].Blocks) != 1 || records[1].Blocks[0].Type != blockTool {
		t.Fatalf("assistant tool record blocks = %#v", records[1].Blocks)
	}

	replayed, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 4 {
		t.Fatalf("replayed messages = %d, want 4", len(replayed))
	}
	if provider.MessageRole(replayed[2]) != "tool" || provider.MessageToolCallID(replayed[2]) != "call_1" {
		t.Fatalf("unexpected replayed tool result %#v", replayed[2])
	}

	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 4 {
		t.Fatalf("mirrored JSON messages = %d, want 4", len(mirrored))
	}
}

func TestImportLegacyJSONCopiesMissingSessions(t *testing.T) {
	root := t.TempDir()
	legacy, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := legacy.CreateSession(storage.CreateSession{Slug: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveMessages(first.ID, []provider.Message{provider.UserMessage("first")}); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	second, err := legacy.CreateSession(storage.CreateSession{Slug: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveMessages(second.ID, []provider.Message{provider.UserMessage("second")}); err != nil {
		t.Fatal(err)
	}

	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.LoadMessages(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || provider.MessageContent(loaded[0]) != "second" {
		t.Fatalf("missing legacy session was not imported: %#v", loaded)
	}
}
