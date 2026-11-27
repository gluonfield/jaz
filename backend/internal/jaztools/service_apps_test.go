package jaztools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
)

func TestSetAppsSyncsAfterServerInitAndStagesDraft(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	events := sessionevents.New()
	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		events,
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testCalendarTools(t, store),
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, fakeWhatsAppProvider{}, fakeWhatsAppProvider{}),
		connections.NewTelegramMCPTools(store, fakeTelegramProvider{}, fakeTelegramProvider{}),
		connections.NewService(connections.NewCatalog(nil), store),
		plugins.New(runtimefiles.New(t.TempDir())),
		visualize.NewService(store, store, events, canvas.NewService(runtimefiles.New(store.RootDir()))),
	)

	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	// Initialize the server slot BEFORE SetApps is called to verify late wiring still exposes prepare_app_draft.
	srv := service.Server()
	appsSvc := apps.NewService(store, store, nil, nil, root, t.TempDir())
	service.SetApps(appsSvc)

	session, closeSession := connectClient(t, srv)
	defer closeSession()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == "prepare_app_draft" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected prepare_app_draft to be registered even when SetApps runs after Server()")
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "prepare_app_draft",
		Arguments: map[string]any{
			"title": "World Clock",
			"html":  "<div><h1>Clock</h1></div>",
		},
	})
	if err != nil || res.IsError || len(res.Content) == 0 {
		t.Fatalf("prepare_app_draft call failed: res=%#v err=%v", res, err)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["draft_id"] == "" || payload["bundle_sha256"] == "" || payload["requires_user_confirmation"] != true {
		t.Fatalf("unexpected prepare_app_draft payload: %#v", payload)
	}
}
