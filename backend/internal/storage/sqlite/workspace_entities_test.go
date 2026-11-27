package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/widgets"
)

func TestWorkspaceEntityIsolationInSQLite(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// 1. Bots + per-workspace pinning.
	botLocal, err := store.CreateSession(storage.CreateSession{Slug: "bot-local", Title: "LocalBot", SourceType: storage.SourceBot, WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	botA, err := store.CreateSession(storage.CreateSession{Slug: "bot-a", Title: "CloudABot", SourceType: storage.SourceBot, WorkspaceID: "cloudA"})
	if err != nil {
		t.Fatal(err)
	}
	botB, err := store.CreateSession(storage.CreateSession{Slug: "bot-b", Title: "CloudBBot", SourceType: storage.SourceBot, WorkspaceID: "cloudB"})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range []storage.BotRecord{
		{ThreadID: botLocal.ID, WorkspaceID: storage.LocalWorkspaceID, Kind: "bot", Shape: "circle", Color: "blue", Main: true},
		{ThreadID: botA.ID, WorkspaceID: "cloudA", Kind: "bot", Shape: "hex", Color: "green", Main: true},
		{ThreadID: botB.ID, WorkspaceID: "cloudB", Kind: "bot", Shape: "cloud", Color: "amber"},
	} {
		if err := store.SaveBot(rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PinBotsForWorkspace(storage.LocalWorkspaceID, []string{botLocal.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.PinBotsForWorkspace("cloudA", []string{botA.ID}); err != nil {
		t.Fatal(err)
	}
	localBots, err := store.ListBotsForWorkspace(storage.LocalWorkspaceID)
	if err != nil || len(localBots) != 1 || localBots[0].ThreadID != botLocal.ID || localBots[0].Pinned != 1 {
		t.Fatalf("local bots = %+v, %v", localBots, err)
	}
	cloudABots, err := store.ListBotsForWorkspace("cloudA")
	if err != nil || len(cloudABots) != 1 || cloudABots[0].ThreadID != botA.ID || cloudABots[0].Pinned != 1 {
		t.Fatalf("cloudA bots = %+v, %v", cloudABots, err)
	}
	cloudBBots, err := store.ListBotsForWorkspace("cloudB")
	if err != nil || len(cloudBBots) != 1 || cloudBBots[0].ThreadID != botB.ID || cloudBBots[0].Pinned != 0 {
		t.Fatalf("cloudB bots = %+v, %v", cloudBBots, err)
	}

	// 2. Boards & Widgets (default Personal board stays strictly in local).
	if _, found, err := store.DefaultBoardForWorkspace(storage.LocalWorkspaceID); err != nil || !found {
		t.Fatalf("local default board found=%v, err=%v", found, err)
	}
	if _, found, err := store.DefaultBoardForWorkspace("cloudA"); err != nil || found {
		t.Fatalf("cloudA unexpectedly inherited default board: found=%v, err=%v", found, err)
	}
	if boards, err := store.ListBoardsForWorkspace("cloudA"); err != nil || len(boards) != 0 {
		t.Fatalf("cloudA initial boards = %+v, %v; want empty", boards, err)
	}
	if err := store.SaveBoard(widgets.Board{ID: "board-a", WorkspaceID: "cloudA", Name: "Ops", GridCols: 6, RowHeight: 120, FontScale: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if boards, err := store.ListBoardsForWorkspace("cloudA"); err != nil || len(boards) != 1 || boards[0].ID != "board-a" {
		t.Fatalf("cloudA boards = %+v, %v", boards, err)
	}
	if boards, err := store.ListBoardsForWorkspace("cloudB"); err != nil || len(boards) != 0 {
		t.Fatalf("cloudB boards = %+v, %v; want empty", boards, err)
	}

	// 3. Loops & Widgets.
	for _, l := range []loops.Loop{
		{ID: "loop-local", WorkspaceID: storage.LocalWorkspaceID, Name: "L", Prompt: "p", Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}, Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now},
		{ID: "loop-a", WorkspaceID: "cloudA", Name: "A", Prompt: "p", Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}, Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.SaveLoop(l); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveWidget(widgets.Widget{ID: "w-local", WorkspaceID: storage.LocalWorkspaceID, LoopID: "loop-local", Title: "WL", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveWidget(widgets.Widget{ID: "w-a", WorkspaceID: "cloudA", LoopID: "loop-a", Title: "WA", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ListLoopsForWorkspace("cloudA"); err != nil || len(got) != 1 || got[0].ID != "loop-a" {
		t.Fatalf("cloudA loops = %+v, %v", got, err)
	}
	if got, err := store.ListLoopsForWorkspace("cloudB"); err != nil || len(got) != 0 {
		t.Fatalf("cloudB loops = %+v, %v", got, err)
	}
	if got, err := store.LoadWidget("w-a"); err != nil || got.WorkspaceID != "cloudA" {
		t.Fatalf("cloudA widget = %+v, %v", got, err)
	}

	// 4. Custom MCP servers.
	if _, err := store.CreateMCPServer(mcpconfig.ServerInput{WorkspaceID: storage.LocalWorkspaceID, Name: "local-mcp", URL: "https://local.example/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateMCPServer(mcpconfig.ServerInput{WorkspaceID: "cloudA", Name: "a-mcp", URL: "https://a.example/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ListMCPServersForWorkspace(storage.LocalWorkspaceID); err != nil || len(got) != 1 || got[0].Name != "local-mcp" {
		t.Fatalf("local mcp = %+v, %v", got, err)
	}
	if got, err := store.ListMCPServersForWorkspace("cloudA"); err != nil || len(got) != 1 || got[0].Name != "a-mcp" {
		t.Fatalf("cloudA mcp = %+v, %v", got, err)
	}
	if got, err := store.ListMCPServersForWorkspace("cloudB"); err != nil || len(got) != 0 {
		t.Fatalf("cloudB mcp = %+v, %v", got, err)
	}

	// 5. Plugin sources & states (same IDs across workspaces without conflict).
	for _, ws := range []string{storage.LocalWorkspaceID, "cloudA"} {
		if err := store.SavePluginSource(ctx, storage.PluginSourceRecord{
			WorkspaceID: ws, ID: "org/plugins", Repository: "org/plugins", URL: "https://example.com/org/plugins.git", CatalogJSON: json.RawMessage(`[]`), UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.SavePluginState(ctx, storage.PluginStateRecord{
			WorkspaceID: ws, PluginID: "tenders", Enabled: ws == "cloudA", UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if sources, err := store.ListPluginSources(ctx, "cloudB"); err != nil || len(sources) != 0 {
		t.Fatalf("cloudB plugin sources = %+v, %v", sources, err)
	}
	localSt, ok, err := store.GetPluginState(ctx, storage.LocalWorkspaceID, "tenders")
	if err != nil || !ok || localSt.Enabled {
		t.Fatalf("local plugin state = %+v, ok=%v, err=%v", localSt, ok, err)
	}
	cloudASt, ok, err := store.GetPluginState(ctx, "cloudA", "tenders")
	if err != nil || !ok || !cloudASt.Enabled {
		t.Fatalf("cloudA plugin state = %+v, ok=%v, err=%v", cloudASt, ok, err)
	}

	// 6. Search, Feed, and Archived sessions (including children).
	tLocal, err := store.CreateSession(storage.CreateSession{Slug: "t-local", Title: "SharedKeyword Local", WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	tCloudA, err := store.CreateSession(storage.CreateSession{Slug: "t-a", Title: "SharedKeyword CloudA", WorkspaceID: "cloudA"})
	if err != nil {
		t.Fatal(err)
	}
	tCloudB, err := store.CreateSession(storage.CreateSession{Slug: "t-b", Title: "SharedKeyword CloudB", WorkspaceID: "cloudB"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []storage.Session{tLocal, tCloudA, tCloudB} {
		if err := store.AppendSessionEvents(s.ID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "Reply with SharedKeyword", At: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.CompleteSession(s.ID, now); err != nil {
			t.Fatal(err)
		}
	}

	// Feed isolation.
	if feedA, err := store.LoadFeedForWorkspace("cloudA"); err != nil || len(feedA) != 1 || feedA[0].ID != tCloudA.ID {
		t.Fatalf("cloudA feed = %+v, %v", feedA, err)
	}
	if feedB, err := store.LoadFeedForWorkspace("cloudB"); err != nil || len(feedB) != 1 || feedB[0].ID != tCloudB.ID {
		t.Fatalf("cloudB feed = %+v, %v", feedB, err)
	}

	// Search isolation.
	activeWS := storage.LocalWorkspaceID
	threadSvc := threads.NewService(NewSearchQueries(store), store).WithWorkspace(func() string { return activeWS })
	if res, err := threadSvc.Search(ctx, threads.SearchQuery{Query: "SharedKeyword"}); err != nil || len(res) != 1 || res[0].ThreadID != tLocal.ID {
		t.Fatalf("local search = %+v, %v", res, err)
	}
	activeWS = "cloudA"
	if res, err := threadSvc.Search(ctx, threads.SearchQuery{Query: "SharedKeyword"}); err != nil || len(res) != 1 || res[0].ThreadID != tCloudA.ID {
		t.Fatalf("cloudA search = %+v, %v", res, err)
	}

	// Archived threads & ListChildSessions isolation.
	if err := store.SetArchived(tLocal.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(tCloudA.ID, true); err != nil {
		t.Fatal(err)
	}
	if archivedA, err := store.ListSessions(storage.SessionFilter{WorkspaceID: "cloudA", Archived: true}); err != nil || len(archivedA) != 1 || archivedA[0].ID != tCloudA.ID {
		t.Fatalf("cloudA archived sessions = %+v, %v", archivedA, err)
	}
	if archivedB, err := store.ListSessions(storage.SessionFilter{WorkspaceID: "cloudB", Archived: true}); err != nil || len(archivedB) != 0 {
		t.Fatalf("cloudB archived sessions = %+v, %v", archivedB, err)
	}
	childLocal, err := store.CreateSession(storage.CreateSession{Slug: "c-local", Title: "Child", ParentID: tLocal.ID, WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if kids, err := store.ListSessions(storage.SessionFilter{WorkspaceID: "cloudA", ParentID: tLocal.ID, ParentOnly: true}); err != nil || len(kids) != 0 {
		t.Fatalf("cloudA leaked local child %s: %+v, %v", childLocal.ID, kids, err)
	}
}
