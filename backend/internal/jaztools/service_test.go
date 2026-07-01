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
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsertask"
	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakeScheduler struct{}

func (fakeScheduler) Start()        {}
func (fakeScheduler) Stop()         {}
func (fakeScheduler) Running() bool { return true }

func testGmailTools(t *testing.T, store connections.GmailToolStore) *connections.GmailMCPTools {
	t.Helper()
	return connections.NewGmailMCPTools(store, integrationingest.RawWriter{Root: t.TempDir()})
}

type fakeExecutor struct {
	started chan loops.Run
}

func (f *fakeExecutor) StartLoopRun(_ context.Context, execution loops.Execution) {
	f.started <- execution.Run
}

type fakeACPService struct {
	spawned chan acp.SpawnRequest
}

type fakeBrowserBackend struct{}

type fakeWhatsAppSender struct{}
type fakeWhatsAppProvider struct{}
type fakeTelegramProvider struct{}

func (s fakeACPService) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	s.spawned <- req
	return acp.SpawnResult{Status: "ok", SessionID: "child", Slug: req.Slug, ACPAgent: req.ACPAgent, State: acp.StateIdle}, nil
}

func (s fakeACPService) Send(context.Context, acp.SendRequest) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) Status(string) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) Wait(context.Context, acp.WaitRequest) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) Cancel(context.Context, string) (acp.Job, error) {
	return acp.Job{}, nil
}

func (s fakeACPService) List() []acp.Job {
	return nil
}

func (s fakeACPService) Agents() []string {
	return []string{acp.AgentCodex, acp.AgentJaz}
}

func (fakeBrowserBackend) Call(context.Context, browserworker.ActionInput) (browserworker.ActionOutput, error) {
	return browserworker.ActionOutput{Status: "ok", Text: "fake browser"}, nil
}

func (fakeBrowserBackend) Status() browserworker.ExtensionStatus {
	return browserworker.ExtensionStatus{Connected: true}
}

func (fakeWhatsAppSender) SendMessage(context.Context, whatsapp.SendMessageRequest) (whatsapp.SendMessageResult, error) {
	return whatsapp.SendMessageResult{}, nil
}

func (fakeWhatsAppProvider) SendMessage(context.Context, whatsapp.SendMessageRequest) (whatsapp.SendMessageResult, error) {
	return whatsapp.SendMessageResult{}, nil
}

func (fakeWhatsAppProvider) Search(context.Context, whatsapp.SearchRequest) (whatsapp.SearchResult, error) {
	return whatsapp.SearchResult{}, nil
}

func (fakeTelegramProvider) SendMessage(context.Context, telegram.SendMessageRequest) (telegram.SendMessageResult, error) {
	return telegram.SendMessageResult{}, nil
}

func (fakeTelegramProvider) Search(context.Context, telegram.SearchRequest) (telegram.SearchResult, error) {
	return telegram.SearchResult{}, nil
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
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgetService, Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, fakeWhatsAppProvider{}, fakeWhatsAppProvider{}),
		connections.NewTelegramMCPTools(store, fakeTelegramProvider{}, fakeTelegramProvider{}),
	)
	executor := &fakeExecutor{started: make(chan loops.Run, 1)}
	service.SetLoops(loops.NewService(store, executor, nil))
	service.SetThreads(threads.NewService(sqlitestore.NewSearchQueries(store), store))
	service.SetAgents(fakeACPService{spawned: make(chan acp.SpawnRequest, 1)})

	target, err := store.CreateSession(storage.CreateSession{Slug: "review-target", Title: "Review target"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(target.ID,
		storage.Message{Role: "user", Content: "Please review the checkout bug."},
		storage.Message{Role: "assistant", Content: "Patched checkout and verified tests."},
	); err != nil {
		t.Fatal(err)
	}

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
		"memory_search", "memory_get_page",
		"thread_context",
		"gmail_get_profile", "gmail_search_threads", "gmail_read_thread", "gmail_create_draft", "gmail_create_reply_draft", "gmail_send_draft", "gmail_update_draft", "gmail_list_drafts", "gmail_read_attachment",
		"whatsapp_search", "whatsapp_send_message", "telegram_search", "telegram_send_message",
		"loop_list", "loop_get", "loop_create", "loop_update", "loop_run", "loop_delete",
		"agent_spawn", "agent_send", "agent_status", "agent_wait", "agent_cancel", "agent_list",
		"create_goal", "get_goal", "update_goal",
		"visualise_read_me", "visualise_show_widget",
	} {
		if !names[name] {
			t.Fatalf("missing tool %s in %#v", name, names)
		}
	}
	if names["visualise_publish_widget"] {
		t.Fatal("visualise_publish_widget must not be advertised on ordinary jaztools sessions")
	}

	threadCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "thread_context",
		Arguments: map[string]any{"session": target.ID, "limit": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	threadContext := structured[threads.ContextResponse](t, threadCall)
	if threadContext.Session.ID != target.ID || len(threadContext.Messages) != 1 || threadContext.Messages[0].Text != "Patched checkout and verified tests." {
		t.Fatalf("thread context = %#v", threadContext)
	}

	readMeCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "visualise_read_me",
		Arguments: map[string]any{"modules": []string{"mockup"}, "platform": "desktop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if readMeCall.IsError || len(readMeCall.Content) == 0 {
		t.Fatalf("read_me result = %#v", readMeCall)
	}

	pageCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_get_page",
		Arguments: map[string]any{"path": "people/alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := structured[struct {
		Found bool   `json:"found"`
		Path  string `json:"path"`
	}](t, pageCall)
	if !page.Found || page.Path != "people/alice" {
		t.Fatalf("page = %#v", page)
	}
	if got := textContent(t, pageCall); got != "# Alice\n\nAlice works on Jaz tools.\n" {
		t.Fatalf("page text = %q", got)
	}

	createCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "loop_create",
		Arguments: map[string]any{
			"name":    "Repo check",
			"prompt":  "check repo health",
			"runtime": "acp",
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

func TestChatToolsFollowRegisteredSenders(t *testing.T) {
	withoutSenders := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	connections.NewWhatsAppMCPTools(nil, nil, nil).AddTo(withoutSenders)
	connections.NewTelegramMCPTools(nil, nil, nil).AddTo(withoutSenders)
	session, closeSession := connectClient(t, withoutSenders)
	defer closeSession()
	if hasTool(t, session, whatsapp.ToolSearch) || hasTool(t, session, whatsapp.ToolSendMessage) ||
		hasTool(t, session, telegram.ToolSearch) || hasTool(t, session, telegram.ToolSendMessage) {
		t.Fatal("chat tools must not be advertised without provider adapters")
	}

	withWhatsApp := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	connections.NewWhatsAppMCPTools(nil, fakeWhatsAppSender{}, nil).AddTo(withWhatsApp)
	session, closeSession = connectClient(t, withWhatsApp)
	defer closeSession()
	if !hasTool(t, session, whatsapp.ToolSendMessage) {
		t.Fatal("whatsapp sender was not advertised")
	}
	if hasTool(t, session, telegram.ToolSendMessage) {
		t.Fatal("telegram sender leaked without adapter")
	}
	if hasTool(t, session, whatsapp.ToolSearch) {
		t.Fatal("whatsapp search leaked from sender-only adapter")
	}

	withSearch := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	whatsappProvider := fakeWhatsAppProvider{}
	connections.NewWhatsAppMCPTools(nil, whatsappProvider, whatsappProvider).AddTo(withSearch)
	session, closeSession = connectClient(t, withSearch)
	defer closeSession()
	if !hasTool(t, session, whatsapp.ToolSearch) || !hasTool(t, session, whatsapp.ToolSendMessage) {
		t.Fatal("whatsapp provider did not advertise search and send")
	}
}

func TestPublishWidgetToolOnlyAdvertisedForWidgetSurfaceSessions(t *testing.T) {
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

	widgetService := widgets.NewService(store, nil)
	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgetService, Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))
	service.SetAgents(fakeACPService{spawned: make(chan acp.SpawnRequest, 1)})

	ordinary, err := store.CreateSession(storage.CreateSession{Slug: "ordinary", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plainLoop := loops.Loop{ID: store.NewLoopID(), Name: "Plain loop", Prompt: "check", Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now}
	plainRun := loops.Run{ID: store.NewRunID(), LoopID: plainLoop.ID, Status: loops.RunStatusRunning, CreatedAt: now}
	if err := store.SaveLoop(plainLoop); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(plainRun); err != nil {
		t.Fatal(err)
	}
	plainLoopRun, err := store.CreateSession(storage.CreateSession{Slug: "plain-loop-run", Runtime: storage.RuntimeACP, SourceType: storage.SourceLoopRun, SourceID: plainRun.ID})
	if err != nil {
		t.Fatal(err)
	}
	widgetLoop := loops.Loop{ID: store.NewLoopID(), Name: "Widget loop", Prompt: "update", Status: loops.StatusActive, Runtime: loops.RuntimeACP, CreatedAt: now, UpdatedAt: now}
	widgetRun := loops.Run{ID: store.NewRunID(), LoopID: widgetLoop.ID, Status: loops.RunStatusRunning, CreatedAt: now}
	if err := store.SaveLoop(widgetLoop); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(widgetRun); err != nil {
		t.Fatal(err)
	}
	loopRun, err := store.CreateSession(storage.CreateSession{
		Slug:       "loop-run",
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceLoopRun,
		SourceID:   widgetRun.ID,
		RuntimeRef: &storage.RuntimeRef{
			Type:            storage.RuntimeACP,
			ArtifactSurface: string(visualize.SurfaceWidget),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.widgetSession(sessionRequest(ordinary.ID)) {
		t.Fatal("ordinary session treated as a loop run")
	}
	if service.widgetSession(sessionRequest(plainLoopRun.ID)) {
		t.Fatal("loop run without board widget enabled widget publishing")
	}
	board, err := widgetService.CreateBoard("Desk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := widgetService.AssignLoopBoards(widgetLoop, []string{board.ID}); err != nil {
		t.Fatal(err)
	}
	if !service.widgetSession(sessionRequest(loopRun.ID)) {
		t.Fatal("assigned widget loop-run session did not enable widget publishing")
	}
	widgetQueryReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/jaztools?jaztools_surface=widget", nil)
	if service.surface(widgetQueryReq) != widgetSurface {
		t.Fatal("widget surface query did not route to widget surface")
	}

	base, closeBase := connectClient(t, service.Server())
	defer closeBase()
	if hasTool(t, base, "visualise_publish_widget") {
		t.Fatal("base server advertised visualise_publish_widget")
	}
	if !hasTool(t, base, "agent_spawn") {
		t.Fatal("base server did not advertise agent_spawn")
	}
	widget, closeWidget := connectClient(t, service.server(widgetSurface))
	defer closeWidget()
	if !hasTool(t, widget, "visualise_read_me") {
		t.Fatal("widget server did not advertise visualise_read_me")
	}
	if hasTool(t, widget, "visualise_show_widget") {
		t.Fatal("widget server advertised thread artifact renderer")
	}
	if !hasTool(t, widget, "agent_spawn") {
		t.Fatal("widget server did not advertise agent_spawn")
	}
	if !hasTool(t, widget, "visualise_publish_widget") {
		t.Fatal("widget server did not advertise visualise_publish_widget")
	}
}

func TestSourceWorkerSurfaceIsRestrictedToMemoryTools(t *testing.T) {
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
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	// Configure the full kit so the absence of these tools proves a real
	// restriction, not just unconfigured services.
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))
	service.SetThreads(threads.NewService(sqlitestore.NewSearchQueries(store), store))
	service.SetAgents(fakeACPService{spawned: make(chan acp.SpawnRequest, 1)})

	// Both routes (session source type and the acp-emitted query param) must
	// resolve to the restricted source surface.
	srcSession, err := store.CreateSession(storage.CreateSession{
		Slug:       "memory-source",
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceMemorySource,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.surface(sessionRequest(srcSession.ID)) != sourceWorkerSurface {
		t.Fatal("memory-source session did not route to source worker surface")
	}
	queryReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/jaztools?jaztools_surface=memory_source_worker", nil)
	if service.surface(queryReq) != sourceWorkerSurface {
		t.Fatal("memory-source surface query did not route to source worker surface")
	}

	source, closeSource := connectClient(t, service.server(sourceWorkerSurface))
	defer closeSource()

	for _, name := range []string{"memory_search", "memory_get_page"} {
		if !hasTool(t, source, name) {
			t.Fatalf("source worker surface missing %s", name)
		}
	}
	for _, name := range []string{"agent_spawn", "thread_context", "gmail_search_threads", "loop_list", "visualise_read_me"} {
		if hasTool(t, source, name) {
			t.Fatalf("source worker surface must not advertise %s", name)
		}
	}
}

func TestWidgetSurfaceGetsAgentToolsAfterServerCreated(t *testing.T) {
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
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	widget, closeWidget := connectClient(t, service.server(widgetSurface))
	defer closeWidget()
	if hasTool(t, widget, "agent_spawn") {
		t.Fatal("widget server advertised agent_spawn before agents were configured")
	}

	service.SetAgents(fakeACPService{spawned: make(chan acp.SpawnRequest, 1)})
	if !hasTool(t, widget, "agent_spawn") {
		t.Fatal("widget server did not advertise agent_spawn after agents were configured")
	}
}

func TestAgentSpawnToolSchemaAndAlias(t *testing.T) {
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
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))
	agentService := fakeACPService{spawned: make(chan acp.SpawnRequest, 1)}
	service.SetAgents(agentService)

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	tool := findTool(t, session, "agent_spawn")
	if tool == nil {
		t.Fatal("agent_spawn not advertised")
	}
	schema, _ := tool.InputSchema.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range []string{"acp_agent", "agent_name", "model_provider", "model", "reasoning_effort"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("agent_spawn schema missing %s: %#v", name, properties)
		}
	}

	call, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "agent_spawn",
		Arguments: map[string]any{
			"agent_name":       acp.AgentCodex,
			"slug":             "child",
			"model_provider":   "openai",
			"model":            "gpt-5.5",
			"reasoning_effort": "high",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.IsError {
		t.Fatalf("agent_spawn returned error: %#v", call)
	}
	select {
	case req := <-agentService.spawned:
		if req.ACPAgent != acp.AgentCodex || req.ModelProvider != "openai" || req.Model != "gpt-5.5" || req.ReasoningEffort != "high" {
			t.Fatalf("spawn request = %#v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("agent_spawn did not reach ACP service")
	}
}

func TestSearchWorkerSurfaceOnlyAdvertisesRawMemoryTools(t *testing.T) {
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

	widgetService := widgets.NewService(store, nil)
	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgetService, Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	searchSession, err := store.CreateSession(storage.CreateSession{
		Slug:       "memory-search",
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceMemorySearch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.surface(sessionRequest(searchSession.ID)) != searchWorkerSurface {
		t.Fatal("memory-search session did not route to search worker surface")
	}
	queryReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/jaztools?jaztools_surface=memory_search_worker", nil)
	if service.surface(queryReq) != searchWorkerSurface {
		t.Fatal("memory-search surface query did not route to search worker surface")
	}

	worker, closeWorker := connectClient(t, service.server(searchWorkerSurface))
	defer closeWorker()
	for _, name := range []string{"jazmem_search_raw", "jazmem_get_page"} {
		if !hasTool(t, worker, name) {
			t.Fatalf("worker server missing %s", name)
		}
	}
	for _, name := range []string{"memory_search", "memory_get_page", "thread_context", "loop_list", "agent_spawn", "visualise_read_me", "visualise_show_widget", "visualise_publish_widget"} {
		if hasTool(t, worker, name) {
			t.Fatalf("worker server advertised %s", name)
		}
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
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	if hasTool(t, session, "memory_get_page") {
		t.Fatal("memory tool advertised while memory is disabled")
	}
	if !hasTool(t, session, "loop_list") {
		t.Fatal("loop tools should remain available")
	}
	widgetSession, closeWidgetSession := connectClient(t, service.server(widgetSurface))
	defer closeWidgetSession()
	if hasTool(t, widgetSession, "memory_get_page") {
		t.Fatal("widget memory tool advertised while memory is disabled")
	}

	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	service.Sync()
	if !hasTool(t, session, "memory_get_page") {
		t.Fatal("memory tool not advertised after memory was enabled")
	}
	if !hasTool(t, widgetSession, "memory_get_page") {
		t.Fatal("widget memory tool not advertised after memory was enabled")
	}

	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	service.Sync()
	if hasTool(t, session, "memory_get_page") {
		t.Fatal("memory tool still advertised after memory was disabled")
	}
	if hasTool(t, widgetSession, "memory_get_page") {
		t.Fatal("widget memory tool still advertised after memory was disabled")
	}
}

func TestBrowserToolsAndWorkerSurface(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := jazsettings.SaveBrowserSettings(store, jazsettings.BrowserSettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		acp.AgentCodex: {Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil))
	service.SetBrowser(browsertask.New(store, fakeACPService{spawned: make(chan acp.SpawnRequest, 1)}, acp.BuiltinAgents(), fakeBrowserBackend{}), fakeBrowserBackend{})

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	for _, name := range []string{"browser_do", "browser_get", "browser_check"} {
		if !hasTool(t, session, name) {
			t.Fatalf("ordinary server missing %s", name)
		}
	}

	browserSession, err := store.CreateSession(storage.CreateSession{
		Slug:       "browser-linkedin",
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceBrowserTask,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.surface(sessionRequest(browserSession.ID)) != browserWorkerSurface {
		t.Fatal("browser task session did not route to browser worker surface")
	}
	queryReq, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/jaztools?jaztools_surface=browser_worker", nil)
	if service.surface(queryReq) != browserWorkerSurface {
		t.Fatal("browser worker surface query did not route")
	}

	worker, closeWorker := connectClient(t, service.server(browserWorkerSurface))
	defer closeWorker()
	if !hasTool(t, worker, "browser") {
		t.Fatal("worker server missing browser")
	}
	for _, name := range []string{"browser_do", "browser_get", "browser_check"} {
		if !hasTool(t, worker, name) {
			t.Fatalf("worker server missing %s", name)
		}
	}
	for _, name := range []string{"memory_search", "agent_spawn", "create_goal", "visualise_read_me"} {
		if hasTool(t, worker, name) {
			t.Fatalf("worker server advertised %s", name)
		}
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
	return findTool(t, session, name) != nil
}

func findTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
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

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	var out string
	for _, content := range res.Content {
		text, ok := content.(*mcp.TextContent)
		if ok {
			out += text.Text
		}
	}
	return out
}
