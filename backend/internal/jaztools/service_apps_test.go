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
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
)

func TestAppToolsSaveDirectlyAndStageElevatedApps(t *testing.T) {
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

	// The server slot exists before SetApps, so late wiring must still expose the app tools.
	srv := service.Server()
	appsSvc := apps.NewService(store, store, nil, nil, root, t.TempDir())
	service.SetApps(appsSvc)

	session, closeSession := connectClient(t, srv)
	defer closeSession()

	call := func(tool string, args map[string]any) (map[string]any, bool) {
		t.Helper()
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || len(res.Content) == 0 {
			t.Fatalf("%s call failed: res=%#v err=%v", tool, res, err)
		}
		var payload map[string]any
		_ = json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &payload)
		return payload, res.IsError
	}

	saved, failed := call("publish_app", map[string]any{"title": "World Clock", "html": "<div><h1>Clock</h1></div>"})
	appID, _ := saved["app_id"].(string)
	app, version, err := appsSvc.GetApp(context.Background(), appID)
	if failed || err != nil || version == nil || app.Title != "World Clock" || app.Visibility != storage.AppVisibilityPrivate || app.CurrentVersion != 1 {
		t.Fatalf("publish_app payload = %#v, app = %#v, version = %#v, err = %v", saved, app, version, err)
	}

	updated, failed := call("publish_app", map[string]any{"title": "World Clock", "html": "<div>v2</div>", "target_app_id": appID})
	if failed || updated["app_id"] != appID || updated["version"] != float64(2) {
		t.Fatalf("publish_app with target_app_id must save version 2 of the same app: %#v", updated)
	}

	elevated := map[string]any{"title": "Summariser", "html": "<div>Summarise</div>", "capabilities": []string{"ai:complete"}}
	if _, failed := call("publish_app", elevated); !failed {
		t.Fatal("publish_app must refuse an app that spends viewer AI credits")
	}
	draft, failed := call("prepare_app_draft", elevated)
	if failed || draft["draft_id"] == "" || draft["requires_user_confirmation"] != true || draft["app_id"] != nil {
		t.Fatalf("elevated app must wait for approval: %#v", draft)
	}
	all, err := appsSvc.ListApps(context.Background(), storage.AppListFilter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("only the approved-free app is saved, got %d (err %v)", len(all), err)
	}
}
