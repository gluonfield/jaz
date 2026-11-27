package apps_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/appbundle"
	"github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

func testBoard(id string) widgets.Board {
	return widgets.Board{
		ID:          id,
		WorkspaceID: storage.LocalWorkspaceID,
		Name:        "Test Board",
		GridCols:    6,
		RowHeight:   120,
		FontScale:   1,
	}
}

type fakeProvider struct {
	reply string
}

func (f fakeProvider) Complete(_ context.Context, _ provider.Request) (provider.Response, error) {
	return provider.Response{
		Message: provider.AssistantMessage(f.reply, nil),
		Usage:   provider.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (f fakeProvider) StreamComplete(_ context.Context, _ provider.Request) (<-chan provider.Event, error) {
	return nil, errors.New("unused")
}

type fakeTools struct {
	lastTool string
}

func (f *fakeTools) CallAppBridgeTool(_ context.Context, name string, _ json.RawMessage) (*mcpsdk.CallToolResult, error) {
	f.lastTool = name
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}, nil
}

func TestLocalAppsLifecycleKVTicketsAIAndBoards(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(root)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	tools := &fakeTools{}
	svc := apps.NewService(store, store, fakeProvider{reply: "hello from ai"}, tools, root, workspace)

	bundle1, err := appbundle.PackFiles(appbundle.Manifest{
		Format:       appbundle.Format,
		Title:        "Budget App",
		Kind:         appbundle.KindArtifact,
		Entry:        "index.html",
		Capabilities: []string{appbundle.CapStoragePersonal, appbundle.CapAIComplete, appbundle.CapToolsCall},
		AllowedTools: []string{"crm_search"},
	}, map[string][]byte{
		"index.html": []byte("<h1>Budget v1</h1>"),
	})
	if err != nil {
		t.Fatalf("PackFiles v1: %v", err)
	}

	draft1, err := svc.StageDraft(ctx, bundle1, "", "")
	if err != nil {
		t.Fatalf("StageDraft: %v", err)
	}

	// Wrong SHA must fail closed.
	if _, _, _, err := svc.PublishFromDraft(ctx, apps.PublishInput{
		RequestedID:  "new",
		DraftID:      draft1.ID,
		BundleSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}); !errors.Is(err, storage.ErrAppDraftMismatch) {
		t.Fatalf("expected ErrAppDraftMismatch, got %v", err)
	}

	app, ver1, isNew, err := svc.PublishFromDraft(ctx, apps.PublishInput{
		RequestedID:  "new",
		DraftID:      draft1.ID,
		BundleSHA256: draft1.BundleSHA256,
		Visibility:   storage.AppVisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("PublishFromDraft v1: %v", err)
	}
	if !isNew || ver1.Version != 1 || app.CurrentVersion != 1 {
		t.Fatalf("unexpected publish result: isNew=%v app=%+v ver=%+v", isNew, app, ver1)
	}

	// Ticket + bundle content serving.
	ticket, err := svc.IssueTicket(ctx, app.ID, 0, "http://127.0.0.1:4242")
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	claims, err := svc.ParseTicket(ticket.Ticket)
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	fileRead, err := svc.ReadBundleFile(ctx, claims, "index.html")
	if err != nil || string(fileRead.Data) != "<h1>Budget v1</h1>" {
		t.Fatalf("ReadBundleFile: %q err=%v", string(fileRead.Data), err)
	}

	// Personal KV works with optimistic concurrency; shared KV is disabled locally.
	if _, _, err := svc.PutKV(ctx, app.ID, storage.AppKVScopeShared, "k", "v", nil); !errors.Is(err, apps.ErrSharedKVDisabled) {
		t.Fatalf("expected ErrSharedKVDisabled, got %v", err)
	}
	kv1, rev1, err := svc.PutKV(ctx, app.ID, storage.AppKVScopePersonal, "theme", "warm", nil)
	if err != nil || rev1 != 1 || kv1.Value != "warm" {
		t.Fatalf("PutKV: %+v rev=%d err=%v", kv1, rev1, err)
	}
	stale := int64(99)
	if _, curRev, err := svc.PutKV(ctx, app.ID, storage.AppKVScopePersonal, "theme", "cool", &stale); !errors.Is(err, storage.ErrAppKVConflict) || curRev != 1 {
		t.Fatalf("expected ErrAppKVConflict with curRev=1, got curRev=%d err=%v", curRev, err)
	}

	// AI completion & strict tool allowlist.
	aiRes, err := svc.AIComplete(ctx, app.ID, apps.AICompleteRequest{Prompt: "hi"})
	if err != nil || aiRes.Text != "hello from ai" {
		t.Fatalf("AIComplete: %+v err=%v", aiRes, err)
	}
	if _, err := svc.CallTool(ctx, app.ID, "not_allowed", nil); !errors.Is(err, apps.ErrToolNotAllowed) {
		t.Fatalf("expected ErrToolNotAllowed, got %v", err)
	}
	if _, err := svc.CallTool(ctx, app.ID, "crm_search", json.RawMessage(`{"q":"test"}`)); err != nil || tools.lastTool != "crm_search" {
		t.Fatalf("CallTool allowed failed: tool=%q err=%v", tools.lastTool, err)
	}

	// Remix creates a new thread with bundle files and zero KV copied.
	remix, err := svc.RemixApp(ctx, app.ID, "")
	if err != nil {
		t.Fatalf("RemixApp: %v", err)
	}
	if remix.Thread.RuntimeRef == nil || remix.Thread.RuntimeRef.Cwd == "" || len(remix.HydratedFiles) < 2 {
		t.Fatalf("unexpected remix: %+v", remix)
	}
	if _, err := os.Stat(filepath.Join(remix.Thread.RuntimeRef.Cwd, "index.html")); err != nil {
		t.Fatalf("expected hydrated index.html: %v", err)
	}

	// Pin onto board & verify ListBoardItems returns it as a tile.
	board, err := store.LoadBoard("missing")
	_ = board
	if err := store.SaveBoard(testBoard("board-1")); err != nil {
		t.Fatalf("SaveBoard: %v", err)
	}
	if err := svc.AssignToBoard(ctx, "board-1", []string{app.ID}); err != nil {
		t.Fatalf("AssignToBoard: %v", err)
	}
	items, err := store.ListBoardItems("board-1")
	if err != nil || len(items) != 1 || items[0].AppID != app.ID || items[0].LoopID != "" || items[0].Title != app.Title {
		t.Fatalf("ListBoardItems: %+v err=%v", items, err)
	}

	// Delete immediately invalidates ticket reads.
	if err := svc.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if _, err := svc.ReadBundleFile(ctx, claims, "index.html"); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("expected deleted app read to fail closed, got %v", err)
	}
}
