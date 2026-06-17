package jaztools

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakeScheduler struct{}

func (fakeScheduler) Start()        {}
func (fakeScheduler) Stop()         {}
func (fakeScheduler) Running() bool { return true }

type fakeExecutor struct {
	started chan loops.Run
}

func (f *fakeExecutor) StartLoopRun(_ context.Context, execution loops.Execution) {
	f.started <- execution.Run
}

func TestUnifiedServerMemoryAndLoopTools(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	if err := os.MkdirAll(filepath.Join(memory.Root(), "people"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memory.Root(), "people", "alice.md"), []byte("# Alice\n\nAlice works on Jaz tools.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Reindex(context.Background(), jazmem.ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	widgetService := widgets.NewService(store, nil)
	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		nil,
		&widgets.SessionPublisher{Service: widgetService, Sessions: store, Loops: store},
	)
	executor := &fakeExecutor{started: make(chan loops.Run, 1)}
	service.SetLoops(loops.NewService(store, executor, nil))

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{
		"memory_search", "memory_get",
		"loop_list", "loop_get", "loop_create", "loop_update", "loop_run", "loop_delete",
		"visualize:read_me", "visualize:show_widget",
	} {
		if !names[name] {
			t.Fatalf("missing tool %s in %#v", name, names)
		}
	}
	if names["publish_widget"] {
		t.Fatal("publish_widget must not be advertised on ordinary jaztools sessions")
	}

	readMeCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "visualize:read_me",
		Arguments: map[string]any{"modules": []string{"mockup"}, "platform": "desktop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if readMeCall.IsError || len(readMeCall.Content) == 0 {
		t.Fatalf("read_me result = %#v", readMeCall)
	}

	pageCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_get",
		Arguments: map[string]any{"slug": "people/alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := structured[struct {
		Found bool   `json:"found"`
		Slug  string `json:"slug"`
		Raw   string `json:"raw"`
	}](t, pageCall)
	if !page.Found || page.Slug != "people/alice" || page.Raw == "" {
		t.Fatalf("page = %#v", page)
	}

	createCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "loop_create",
		Arguments: map[string]any{
			"name":    "Repo check",
			"prompt":  "check repo health",
			"runtime":"acp",
			"schedule": map[string]any{
				"kind":     loops.ScheduleCron,
				"expr":     "0 9 * * *",
				"timezone": "UTC",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := structured[loops.Loop](t, createCall)
	if created.ID == "" || created.Name != "Repo check" || created.Runtime != loops.RuntimeACP {
		t.Fatalf("created = %#v", created)
	}

	listCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "loop_list"})
	if err != nil {
		t.Fatal(err)
	}
	list := structured[loops.MCPListOutput](t, listCall)
	if len(list.Loops) != 1 || list.Loops[0].ID != created.ID {
		t.Fatalf("list = %#v", list)
	}

	paused := loops.StatusPaused
	updateCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "loop_update",
		Arguments: map[string]any{"id": created.ID, "status": paused},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := structured[loops.Loop](t, updateCall)
	if updated.Status != loops.StatusPaused {
		t.Fatalf("updated = %#v", updated)
	}

	getCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "loop_get",
		Arguments: map[string]any{"id": created.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail := structured[loops.MCPDetailOutput](t, getCall)
	if detail.Loop.ID != created.ID {
		t.Fatalf("detail = %#v", detail)
	}

	runCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "loop_run",
		Arguments: map[string]any{"id": created.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := structured[loops.Run](t, runCall)
	select {
	case started := <-executor.started:
		if started.ID != run.ID {
			t.Fatalf("started = %s, run = %s", started.ID, run.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("manual run did not dispatch")
	}

	deleteCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "loop_delete",
		Arguments: map[string]any{"id": created.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted := structured[loops.MCPDeleteOutput](t, deleteCall)
	if !deleted.OK {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestPublishWidgetToolOnlyAdvertisedForLoopSessions(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		nil,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	ordinary, err := store.CreateSession(storage.CreateSession{Slug: "ordinary", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	loopRun, err := store.CreateSession(storage.CreateSession{Slug: "loop-run", Runtime: storage.RuntimeACP, SourceType: storage.SourceLoopRun, SourceID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if service.widgetSession(sessionRequest(ordinary.ID)) {
		t.Fatal("ordinary session treated as a loop run")
	}
	if !service.widgetSession(sessionRequest(loopRun.ID)) {
		t.Fatal("loop-run session did not enable widget publishing")
	}

	base, closeBase := connectClient(t, service.Server())
	defer closeBase()
	if hasTool(t, base, "publish_widget") {
		t.Fatal("base server advertised publish_widget")
	}
	widget, closeWidget := connectClient(t, service.server(widgetSurface))
	defer closeWidget()
	if !hasTool(t, widget, "visualize:read_me") {
		t.Fatal("widget server did not advertise visualize:read_me")
	}
	if hasTool(t, widget, "visualize:show_widget") {
		t.Fatal("widget server advertised thread artifact renderer")
	}
	if !hasTool(t, widget, "publish_widget") {
		t.Fatal("widget server did not advertise publish_widget")
	}
}

func TestMemoryToolsFollowEnabledSetting(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}

	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jazmem"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		nil,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	if hasTool(t, session, "memory_get") {
		t.Fatal("memory tool advertised while memory is disabled")
	}
	if !hasTool(t, session, "loop_list") {
		t.Fatal("loop tools should remain available")
	}
	widgetSession, closeWidgetSession := connectClient(t, service.server(widgetSurface))
	defer closeWidgetSession()
	if hasTool(t, widgetSession, "memory_get") {
		t.Fatal("widget memory tool advertised while memory is disabled")
	}

	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	service.Sync()
	if !hasTool(t, session, "memory_get") {
		t.Fatal("memory tool not advertised after memory was enabled")
	}
	if !hasTool(t, widgetSession, "memory_get") {
		t.Fatal("widget memory tool not advertised after memory was enabled")
	}

	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	service.Sync()
	if hasTool(t, session, "memory_get") {
		t.Fatal("memory tool still advertised after memory was disabled")
	}
	if hasTool(t, widgetSession, "memory_get") {
		t.Fatal("widget memory tool still advertised after memory was disabled")
	}
}

func connectClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatal(err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func hasTool(t *testing.T, session *mcp.ClientSession, name string) bool {
	t.Helper()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func sessionRequest(sessionID string) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/jaztools", nil)
	req.Header.Set(mcpsession.HeaderName, sessionID)
	return req
}

func structured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
