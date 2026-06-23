package threads

import (
	"context"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestContextTailRedactsReasoningAndSummarizesTools(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "child-review", Title: "Child review"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "system", Content: "private system prompt"},
		storage.Message{Role: "user", Content: "Review the payment flow."},
		storage.Message{Role: "assistant", Blocks: []storage.Block{
			{Type: storage.BlockTypeReasoning, Text: "private reasoning"},
			{Type: storage.BlockTypeTool, ID: "tool-1", Name: "exec", InputJSON: `{"cmd":"cat secret"}`, Result: strings.Repeat("secret output ", 80)},
		}},
		storage.Message{Role: "assistant", Content: "Found the checkout regression and patched it."},
	); err != nil {
		t.Fatal(err)
	}

	got, err := NewService(sqlitestore.NewSearchQueries(store), store).Context(context.Background(), ContextRequest{
		Session: session.ID,
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.ID != session.ID || got.Session.MessageCount != 3 {
		t.Fatalf("session summary = %#v", got.Session)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages = %#v, want user, tool summary, assistant", got.Messages)
	}
	if got.Messages[1].Role != "assistant" || len(got.Messages[1].Tools) != 1 || got.Messages[1].Tools[0].Name != "exec" {
		t.Fatalf("tool summary message = %#v", got.Messages[1])
	}
	body := joinedContextText(got.Messages)
	for _, leaked := range []string{"private system", "private reasoning", "secret output", `"cmd"`} {
		if strings.Contains(body, leaked) {
			t.Fatalf("context leaked %q in %q", leaked, body)
		}
	}
	if got.ToolCounts["exec"] != 1 {
		t.Fatalf("tool counts = %#v", got.ToolCounts)
	}
}

func TestContextQueryMatchesToolOutputWithoutReturningIt(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "test-run"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "user", Content: "Run the tests."},
		storage.Message{Role: "assistant", Blocks: []storage.Block{
			{Type: storage.BlockTypeTool, ID: "tool-1", Name: "exec", InputJSON: `{"cmd":"go test ./..."}`, Result: "FAIL payment TestCheckout panic: nil cart"},
		}},
		storage.Message{Role: "assistant", Content: "I fixed the TestCheckout nil cart panic."},
	); err != nil {
		t.Fatal(err)
	}

	got, err := NewService(sqlitestore.NewSearchQueries(store), store).Context(context.Background(), ContextRequest{
		Session: session.ID,
		Query:   "TestCheckout panic",
		Context: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "query" || got.MatchCount != 2 {
		t.Fatalf("query response = %#v", got)
	}
	if len(got.Messages) != 2 || !got.Messages[0].Matched || !got.Messages[1].Matched {
		t.Fatalf("query messages = %#v", got.Messages)
	}
	body := joinedContextText(got.Messages)
	if strings.Contains(body, "FAIL payment") || strings.Contains(body, "go test") {
		t.Fatalf("query returned raw tool detail: %q", body)
	}
}

func TestContextPaginationAndTextClamp(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "paging"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two", "three", "four", strings.Repeat("x", 20)} {
		if err := store.AppendMessageRecords(session.ID, storage.Message{Role: "user", Content: text}); err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(sqlitestore.NewSearchQueries(store), store)
	got, err := service.Context(context.Background(), ContextRequest{
		Session:      session.ID,
		BeforeSeq:    5,
		Limit:        2,
		MaxTextChars: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "before" || len(got.Messages) != 2 || got.Messages[0].Seq != 3 || got.Messages[1].Seq != 4 {
		t.Fatalf("page = %#v", got)
	}
	if !got.HasMoreBefore || !got.HasMoreAfter || got.NextBeforeSeq != 3 || got.NextAfterSeq != 4 {
		t.Fatalf("page cursors = %#v", got)
	}

	got, err = service.Context(context.Background(), ContextRequest{
		Session:      session.ID,
		AroundSeq:    5,
		Limit:        1,
		MaxTextChars: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Text != "xxxx..." || !got.Messages[0].Truncated {
		t.Fatalf("clamped around page = %#v", got.Messages)
	}
}

func joinedContextText(messages []ContextMessage) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Text)
		for _, tool := range message.Tools {
			b.WriteString(tool.Name)
			b.WriteString(tool.Detail)
		}
	}
	return b.String()
}
