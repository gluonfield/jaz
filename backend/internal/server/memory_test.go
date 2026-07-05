package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/jaztools"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakeMemoryScheduler struct {
	running bool
}

func (f *fakeMemoryScheduler) Start()        { f.running = true }
func (f *fakeMemoryScheduler) Stop()         { f.running = false }
func (f *fakeMemoryScheduler) Running() bool { return f.running }

func testMemoryServer(t *testing.T) (*Server, *fakeMemoryScheduler) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	sourceProjectionQueue := sourcequeue.New(t.TempDir())
	memorySourceQueue := sourcequeue.New(t.TempDir())
	scheduler := &fakeMemoryScheduler{running: true}
	svc := memoryservice.New(memory, store, scheduler, "http://127.0.0.1:5299/mcp/jazmem")
	widgetService := widgets.NewService(store, nil)
	publisher := &widgets.SessionPublisher{Service: widgetService, Sessions: store, Loops: store}
	events := sessionevents.New()
	tools := jaztools.New(
		svc,
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		events,
		store,
		store,
		publisher,
		connections.NewCalendarMCPTools(store),
		connections.NewGmailMCPTools(store, integrationingest.RawWriter{Root: t.TempDir()}),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	return &Server{
		Store:                 store,
		Memory:                svc,
		JazTools:              tools,
		ModelCatalog:          modelcatalog.NewService(nil),
		SourceProjectionQueue: sourceProjectionQueue,
		MemorySourceQueue:     memorySourceQueue,
	}, scheduler
}

func TestMemoryStatusAndToggle(t *testing.T) {
	srv, scheduler := testMemoryServer(t)
	ctx := context.Background()
	sourceProjectionQueue := srv.SourceProjectionQueue.(*sourcequeue.Queue)
	if err := sourceProjectionQueue.MarkPendingSource(ctx, sourcequeue.Source{
		Path:     "gmail/personal/messages/2026/06/28/a.md",
		Provider: "gmail",
		Kind:     "message",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceProjectionQueue.Reserve(ctx, 1); err != nil {
		t.Fatal(err)
	}
	memorySourceQueue := srv.MemorySourceQueue.(*sourcequeue.Queue)
	if err := memorySourceQueue.MarkPendingSource(ctx, sourcequeue.Source{
		Path:     "sources/gmail/personal/messages/2026/06/28/a.md",
		Provider: "gmail",
		Kind:     "message",
	}); err != nil {
		t.Fatal(err)
	}
	handler := srv.Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/memory", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var status memoryStatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.SchedulerRunning || status.MCPURL == "" {
		t.Fatalf("unexpected default status %#v", status)
	}
	if status.Agent != "" {
		t.Fatalf("unexpected default memory agent %q", status.Agent)
	}
	if len(status.Horizons) != 2 || status.Horizons[0].Name != jazmem.LongTermFile || status.Horizons[0].Chars == 0 {
		t.Fatalf("unexpected horizons %#v", status.Horizons)
	}
	if len(status.Tasks) != 5 {
		t.Fatalf("unexpected scheduler tasks %#v", status.Tasks)
	}
	byTaskName := map[string]bool{}
	for _, task := range status.Tasks {
		byTaskName[task.Name] = true
	}
	for _, name := range []string{jazmem.TaskIndexChangedPages, jazmem.TaskDailyRollup, jazmem.TaskLinkHygiene, jazmem.TaskDream, jazmem.TaskOptimizeIndex} {
		if !byTaskName[name] {
			t.Fatalf("missing scheduler task %s in %#v", name, status.Tasks)
		}
	}
	if status.SourceQueues.Projection.Pending != 0 || status.SourceQueues.Projection.Processing != 1 {
		t.Fatalf("unexpected source projection queue status %#v", status.SourceQueues.Projection)
	}
	if status.SourceQueues.Memory.Pending != 1 || status.SourceQueues.Memory.Processing != 0 {
		t.Fatalf("unexpected memory source queue status %#v", status.SourceQueues.Memory)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"enabled":false}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled || scheduler.running {
		t.Fatalf("disable should stop scheduler, got %#v running=%v", status, scheduler.running)
	}
	if srv.Memory.Enabled() {
		t.Fatalf("memory should be disabled after toggle")
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"enabled":true}`)))
	if res.Code != http.StatusOK || !scheduler.running {
		t.Fatalf("re-enable failed: %d running=%v", res.Code, scheduler.running)
	}
}

func TestMemoryAgentSetting(t *testing.T) {
	srv, _ := testMemoryServer(t)
	handler := srv.Handler()

	if _, err := jazsettings.SaveAgentDefaults(srv.Store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		"codex":  {Enabled: true, Command: "codex-acp"},
		"claude": {Enabled: true, Command: "claude-acp"},
	}}); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"agent":"codex"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("set memory agent = %d, body = %s", res.Code, res.Body.String())
	}
	var status memoryStatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != "codex" || !status.Enabled {
		t.Fatalf("unexpected memory agent status %#v", status)
	}
	if status.Model != "" || status.DefaultModel != "gpt-5.4-mini" || status.DefaultReasoningEffort != "low" {
		t.Fatalf("unexpected memory worker defaults %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"model":"gpt-5.5","reasoning_effort":"high"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("set memory model = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Model != "gpt-5.5" || status.ReasoningEffort != "high" {
		t.Fatalf("unexpected memory model status %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"reasoning_effort":"max"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unsupported codex effort should 400, got %d body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"agent":"claude"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("switch memory agent = %d, body = %s", res.Code, res.Body.String())
	}
	status = memoryStatusResponse{}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != "claude" || status.Model != "" || status.ReasoningEffort != "" || status.DefaultModel != "sonnet" {
		t.Fatalf("switching agents should reset overrides, got %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"agent":"codex"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("restore memory agent = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"enabled":false}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("toggle with memory agent = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != "codex" || status.Enabled {
		t.Fatalf("toggle should preserve memory agent and disable memory, got %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"agent":"missing"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown memory agent should 400, got %d body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"agent":"jaz"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("built-in Jaz memory agent should 400, got %d body = %s", res.Code, res.Body.String())
	}
}

func TestMemoryHorizonWriteAndReindex(t *testing.T) {
	srv, _ := testMemoryServer(t)
	handler := srv.Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory/horizons/LONG_TERM.md", strings.NewReader(`{"content":"# Long Term Memory\n\n- Goal: $5m."}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("horizon write = %d, body = %s", res.Code, res.Body.String())
	}
	var horizon memoryHorizon
	if err := json.Unmarshal(res.Body.Bytes(), &horizon); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(horizon.Content, "$5m") || horizon.Chars == 0 {
		t.Fatalf("unexpected horizon %#v", horizon)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/memory/horizons/AGENTS.md", strings.NewReader(`{"content":"x"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("non-horizon write should 400, got %d", res.Code)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/memory/reindex", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "page_count") {
		t.Fatalf("reindex = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestMemoryReindexIndexesProviderSourceFiles(t *testing.T) {
	srv, _ := testMemoryServer(t)
	root := srv.Memory.Root()
	sourceTerms := map[string]string{
		"sources/gmail/personal/messages/2026/06/28/gmail-message":                       "gmailmaterializedanchor",
		"sources/telegram/personal/conversations/user-123/2026/06/28":                    "telegrammaterializedanchor",
		"sources/whatsapp/personal/conversations/447700900123@s.whatsapp.net/2026/06/28": "whatsappmaterializedanchor",
	}
	for slug, term := range sourceTerms {
		path := filepath.Join(root, filepath.FromSlash(slug+".md"))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "# Materialized Source\n\nparticipants: Augustinas, Majid <majid@example.com>\n\n" + term + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if _, err := srv.Memory.RunIndexTask(ctx); err != nil {
		t.Fatal(err)
	}
	for slug, term := range sourceTerms {
		results, err := srv.Memory.Search(ctx, term, jazmem.SearchOptions{Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if !serverSlugsContain(results, slug) {
			t.Fatalf("%s was not searchable for %q: %#v", slug, term, results)
		}
	}
}

func TestMemoryDreamEndpointIndexesThenDreams(t *testing.T) {
	srv, _ := testMemoryServer(t)
	root := srv.Memory.Root()
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "projects", "manual-dream.md"), []byte("---\ntitle: Manual Dream\n---\n\n# Manual Dream\n\nManual dream endpoint indexes before running dream.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv.Memory.SetDreamRunner(fakeServerDreamRunner{
		run: func(context.Context, jazmem.DreamRequest) (jazmem.DreamReport, error) {
			return jazmem.DreamReport{RunSlug: "dreams/runs/manual", ModelUsed: "test"}, nil
		},
	})
	handler := srv.Handler()

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/memory/dream", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("dream = %d, body = %s", res.Code, res.Body.String())
	}
	var out jazmem.DreamTaskReport
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Index.PageCount == 0 || out.Dream.RunSlug != "dreams/runs/manual" {
		t.Fatalf("unexpected dream response %#v", out)
	}
	results, err := srv.Memory.Search(context.Background(), "indexes before running dream", jazmem.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !serverSlugsContain(results, "projects/manual-dream") {
		t.Fatalf("manual dream endpoint did not leave index searchable: %#v", results)
	}
	tasks, err := srv.Memory.SchedulerStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]jazmem.TaskStatus{}
	for _, task := range tasks {
		byName[task.Name] = task
	}
	for _, name := range []string{jazmem.TaskIndexChangedPages, jazmem.TaskDream} {
		if byName[name].Status != "ok" || byName[name].LastRunAt.IsZero() {
			t.Fatalf("%s task was not recorded after manual dream: %#v", name, byName[name])
		}
	}
}

func TestMemoryMCPGatedOnSetting(t *testing.T) {
	srv, _ := testMemoryServer(t)
	handler := srv.Handler()

	// Enabled: the real embedded MCP handler answers (any non-gate status).
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/mcp/jazmem", nil))
	if res.Code == http.StatusServiceUnavailable || res.Code == http.StatusNotFound {
		t.Fatalf("enabled mcp should not be gated, got %d %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/jazmem/health", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "ok") {
		t.Fatalf("enabled jazmem api should serve health, got %d %s", res.Code, res.Body.String())
	}

	toggle := httptest.NewRecorder()
	handler.ServeHTTP(toggle, httptest.NewRequest(http.MethodPut, "/v1/memory", strings.NewReader(`{"enabled":false}`)))
	if toggle.Code != http.StatusOK {
		t.Fatalf("toggle failed: %d", toggle.Code)
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/mcp/jazmem", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled mcp should 503, got %d", res.Code)
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/jazmem/health", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled jazmem api should 503, got %d", res.Code)
	}
}

type fakeServerDreamRunner struct {
	run func(context.Context, jazmem.DreamRequest) (jazmem.DreamReport, error)
}

func (f fakeServerDreamRunner) RunDream(ctx context.Context, req jazmem.DreamRequest) (jazmem.DreamReport, error) {
	return f.run(ctx, req)
}

func serverSlugsContain(results []jazmem.Result, slug string) bool {
	for _, result := range results {
		if result.Slug == slug {
			return true
		}
	}
	return false
}
