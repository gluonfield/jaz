package threads

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSearchFindsMessagesAndThreadMetadata(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	thread, err := store.CreateSession(storage.CreateSession{Slug: "matrix-sync", Title: "Matrix sync design"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(thread.ID,
		storage.Message{Role: "user", Content: "Please review the migration plan for Matrix sync."},
		storage.Message{Role: "assistant", Content: "The migration should use append-only records."},
	); err != nil {
		t.Fatal(err)
	}

	archived, err := store.CreateSession(storage.CreateSession{Slug: "archived-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(archived.ID, storage.Message{Role: "user", Content: "Archived migration notes"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(archived.ID, true); err != nil {
		t.Fatal(err)
	}

	search := NewService(sqlitestore.NewSearchQueries(store), store)
	results, err := search.Search(context.Background(), SearchQuery{Query: "migra", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ThreadID != thread.ID {
		t.Fatalf("results = %#v, want only active matrix thread", results)
	}
	if results[0].MessageSeq == 0 {
		t.Fatalf("message hit seq = %d, want message hit", results[0].MessageSeq)
	}
	if !strings.Contains(results[0].Snippet, "\x1fmigration\x1e") {
		t.Fatalf("snippet = %q, want highlighted migration", results[0].Snippet)
	}

	results, err = search.Search(context.Background(), SearchQuery{Query: "matrix sync", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].ThreadID != thread.ID {
		t.Fatalf("metadata search results = %#v, want matrix thread", results)
	}
}

func TestSearchExcludesLoopThreads(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	chat, err := store.CreateSession(storage.CreateSession{Slug: "weather-chat", Title: "Weather report"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(chat.ID, storage.Message{Role: "user", Content: "weather report please"}); err != nil {
		t.Fatal(err)
	}

	loopRun, err := store.CreateSession(storage.CreateSession{
		Slug:       "weather-loop-run",
		Title:      "Weather report loop",
		SourceType: storage.SourceLoopRun,
		SourceID:   "loop-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(loopRun.ID, storage.Message{Role: "user", Content: "weather report please"}); err != nil {
		t.Fatal(err)
	}

	search := NewService(sqlitestore.NewSearchQueries(store), store)

	for _, query := range []string{"weather report", "weather"} {
		results, err := search.Search(context.Background(), SearchQuery{Query: query, IncludeArchived: true, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].ThreadID != chat.ID {
			t.Fatalf("search %q = %#v, want only the chat thread (loop run excluded)", query, results)
		}
	}
}

func TestSearchRanksActiveAboveArchived(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	active, err := store.CreateSession(storage.CreateSession{Slug: "active-deploy", Title: "Deploy pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(active.ID, storage.Message{Role: "user", Content: "deploy the service"}); err != nil {
		t.Fatal(err)
	}

	// The archived thread matches "deploy" far more heavily, so on relevance
	// alone it would outrank the active one.
	archived, err := store.CreateSession(storage.CreateSession{Slug: "archived-deploy", Title: "Deploy deploy deploy runbook"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(archived.ID, storage.Message{Role: "user", Content: "deploy deploy deploy notes"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(archived.ID, true); err != nil {
		t.Fatal(err)
	}

	search := NewService(sqlitestore.NewSearchQueries(store), store)

	// Default search still hides archived threads entirely.
	results, err := search.Search(context.Background(), SearchQuery{Query: "deploy", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ThreadID != active.ID {
		t.Fatalf("default search = %#v, want only active thread", results)
	}

	// Including archived returns both, but the active thread still ranks first.
	results, err = search.Search(context.Background(), SearchQuery{Query: "deploy", IncludeArchived: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want active and archived threads", results)
	}
	if results[0].ThreadID != active.ID || results[0].Archived {
		t.Fatalf("results[0] = %#v, want active thread ranked first", results[0])
	}
	if results[1].ThreadID != archived.ID || !results[1].Archived {
		t.Fatalf("results[1] = %#v, want archived thread ranked last", results[1])
	}
}

func TestSearchIndexMaintenance(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	thread, err := store.CreateSession(storage.CreateSession{Slug: "search-index"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages(thread.ID, []provider.Message{
		provider.UserMessage("ordinary prompt"),
		provider.AssistantMessage("assistant-only quartz finding", nil),
	}); err != nil {
		t.Fatal(err)
	}

	search := NewService(sqlitestore.NewSearchQueries(store), store)
	results, err := search.Search(context.Background(), SearchQuery{Query: "quartz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ThreadID != thread.ID {
		t.Fatalf("search = %#v, want thread hit", results)
	}

	if err := store.SaveMessages(thread.ID, []provider.Message{provider.UserMessage("new vector prompt")}); err != nil {
		t.Fatal(err)
	}
	results, err = search.Search(context.Background(), SearchQuery{Query: "quartz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("stale search hit after replace: %#v", results)
	}
	results, err = search.Search(context.Background(), SearchQuery{Query: "vector"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ThreadID != thread.ID {
		t.Fatalf("new search hit = %#v, want replaced user message", results)
	}
}

func TestSearchHonorsCanceledContext(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = NewService(sqlitestore.NewSearchQueries(store), store).Search(ctx, SearchQuery{Query: "cancelled"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
