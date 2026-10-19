package sqlite

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

// Quote blocks must survive the codec; a missing case in validateBlocks once
// made AppendUserMessage fail silently, dropping the whole user message.
func TestAppendUserMessageWithSelectionContextPersists(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "selection-context"})
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.AppendUserMessage(store, session.ID, "what is this", storage.SelectionContexts([]string{"selected text"}), nil, true); err != nil {
		t.Fatalf("append user message with selection context: %v", err)
	}

	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatalf("load records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Content != "what is this" {
		t.Fatalf("content = %q, want clean user text without the quote", records[0].Content)
	}
	var quote *storage.Block
	for i := range records[0].Blocks {
		if records[0].Blocks[i].Type == storage.BlockTypeQuote {
			quote = &records[0].Blocks[i]
		}
	}
	if quote == nil || quote.Text != "selected text" {
		t.Fatalf("quote block = %#v, blocks = %#v", quote, records[0].Blocks)
	}
}

// A rewritten transcript, as an agent's snapshot after its turn, keeps who
// wrote each stored message: an agent's message never turns into the user's.
func TestSavedTranscriptKeepsWhichMessagesAnAgentWrote(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "agent-message"})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendUserMessage(store, session.ID, "follow up on the notes", nil, nil, false); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveMessages(session.ID, []provider.Message{provider.UserMessage("follow up on the notes"), provider.AssistantMessage("done", nil), provider.UserMessage("thanks")}); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].FromUser || records[2].FromUser {
		t.Fatalf("records = %+v, want no inferred human authorship", records)
	}
}

func TestOnlyExplicitHumanMessagesCountAsUserActivity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "authorship"})
	if err != nil {
		t.Fatal(err)
	}
	assertLast := func(want time.Time) {
		t.Helper()
		last, err := store.LastUserMessageAt()
		if err != nil || !last.Equal(want) {
			t.Fatalf("last user activity = %v, %v, want %v", last, err, want)
		}
	}
	if err := store.AppendMessageRecords(session.ID, storage.Message{Role: "user", Content: "unmarked"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessages(session.ID, provider.UserMessage("provider message")); err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendUserMessage(store, session.ID, "agent", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	assertLast(time.Time{})
	if err := storage.AppendUserMessage(store, session.ID, "human", nil, nil, true); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	wrote := records[len(records)-1].CreatedAt
	assertLast(wrote)
	messages, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages = append(messages, provider.UserMessage("snapshot addition"))
	if err := store.SaveMessages(session.ID, messages); err != nil {
		t.Fatal(err)
	}
	assertLast(wrote)
	messages[len(messages)-2] = provider.UserMessage("replacement without authorship")
	if err := store.SaveMessages(session.ID, messages); err != nil {
		t.Fatal(err)
	}
	assertLast(time.Time{})
}
