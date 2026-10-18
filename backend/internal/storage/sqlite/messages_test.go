package sqlite

import (
	"testing"

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

	if err := storage.AppendUserMessage(store, session.ID, "what is this", storage.SelectionContexts([]string{"selected text"}), nil, false); err != nil {
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
	if err := storage.AppendUserMessage(store, session.ID, "follow up on the notes", nil, nil, true); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveMessages(session.ID, []provider.Message{provider.UserMessage("follow up on the notes"), provider.AssistantMessage("done", nil), provider.UserMessage("thanks")}); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadMessageRecords(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || !records[0].FromAgent || records[2].FromAgent {
		t.Fatalf("records = %+v, want only the first written by an agent", records)
	}
}
