package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

// Quote blocks must survive the codec; a missing case in validateBlocks once
// made AppendUserMessage fail silently, dropping the whole user message.
func TestAppendUserMessageWithQuotesPersists(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "quotes"})
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.AppendUserMessage(store, session.ID, "what is this", []string{"selected text"}, nil); err != nil {
		t.Fatalf("append user message with quotes: %v", err)
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
