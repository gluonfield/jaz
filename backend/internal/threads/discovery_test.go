package threads

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestMCPDiscoveryIncludesBotWorkersWithoutSidebarExposure(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	worker, err := store.CreateSession(storage.CreateSession{
		Slug: "us-mfg-opportunities", Title: "Manufacturing opportunities", ParentID: "bot",
		SourceType: storage.SourceBotWorker, SourceID: "bot",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "claude"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(worker.ID, storage.Message{Role: "user", Content: "Investigate stamping."}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(worker.ID, sessionevents.Event{
		Type: sessionevents.TypeACPMessage, Content: "Foundry report ready.",
		ACP: &sessionevents.ACPEvent{ID: worker.ID, TextRunID: "answer"},
	}); err != nil {
		t.Fatal(err)
	}
	service := NewService(sqlitestore.NewSearchQueries(store), store)
	for _, archived := range []bool{false, true} {
		if err := store.SetArchived(worker.ID, archived); err != nil {
			t.Fatal(err)
		}
		_, listed, err := service.ListMCP(t.Context(), nil, ListRequest{Archived: archived, Limit: 1})
		if err != nil || len(listed.Threads) != 1 || listed.Threads[0].ThreadID != worker.ID || listed.Threads[0].Agent != "claude" {
			t.Fatalf("list archived=%v = %+v, %v", archived, listed, err)
		}
		for _, query := range []string{"us mfg opportunities", "stamping", "foundry"} {
			_, found, err := service.SearchMCP(t.Context(), nil, SearchRequest{Query: query, IncludeArchived: archived, Limit: 1})
			if err != nil || len(found.Threads) != 1 || found.Threads[0].ThreadID != worker.ID {
				t.Fatalf("search %q archived=%v = %+v, %v", query, archived, found, err)
			}
			sidebar, err := service.Search(t.Context(), SearchQuery{Query: query, IncludeArchived: archived})
			if err != nil || len(sidebar) != 0 {
				t.Fatalf("sidebar search exposed worker: %+v, %v", sidebar, err)
			}
		}
		sidebar, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, Archived: archived})
		if err != nil || len(sidebar) != 0 {
			t.Fatalf("sidebar list exposed worker: %+v, %v", sidebar, err)
		}
	}
}
