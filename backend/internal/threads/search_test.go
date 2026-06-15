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

	search := NewService(sqlitestore.NewSearchQueries(store))
	results, err := search.Search(context.Background(), SearchQuery{Query: "migra", Roles: []SearchRole{SearchRoleUser}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ThreadID != thread.ID {
		t.Fatalf("results = %#v, want only active matrix thread", results)
	}
	if results[0].Role != "user" || results[0].MessageSeq == 0 {
		t.Fatalf("message hit = role %q seq %d, want user message", results[0].Role, results[0].MessageSeq)
	}
	if !strings.Contains(results[0].Snippet, "\x1fmigration\x1e") {
		t.Fatalf("snippet = %q, want highlighted migration", results[0].Snippet)
	}

	results, err = search.Search(context.Background(), SearchQuery{Query: "matrix sync", Roles: []SearchRole{SearchRoleAssistant}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].ThreadID != thread.ID {
		t.Fatalf("metadata search results = %#v, want matrix thread", results)
	}
}

func TestSearchRoleFilterAndIndexMaintenance(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	thread, err := store.CreateSession(storage.CreateSession{Slug: "roles"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages(thread.ID, []provider.Message{
		provider.UserMessage("ordinary prompt"),
		provider.AssistantMessage("assistant-only quartz finding", nil),
	}); err != nil {
		t.Fatal(err)
	}

	search := NewService(sqlitestore.NewSearchQueries(store))
	results, err := search.Search(context.Background(), SearchQuery{Query: "quartz", Roles: []SearchRole{SearchRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("user-only search found assistant hit: %#v", results)
	}

	results, err = search.Search(context.Background(), SearchQuery{Query: "quartz", Roles: []SearchRole{SearchRoleAssistant}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Role != "assistant" {
		t.Fatalf("assistant search = %#v, want assistant hit", results)
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

	_, err = NewService(sqlitestore.NewSearchQueries(store)).Search(ctx, SearchQuery{Query: "cancelled"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
