package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/jaztools"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
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
	scheduler := &fakeMemoryScheduler{running: true}
	svc := memoryservice.New(memory, store, scheduler, "http://127.0.0.1:5299/mcp/jazmem")
	tools := jaztools.New(svc, serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"}, store, nil)
	return &Server{Store: store, Memory: svc, JazTools: tools}, scheduler
}

func TestMemoryStatusAndToggle(t *testing.T) {
	srv, scheduler := testMemoryServer(t)
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
	if len(status.Horizons) != 2 || status.Horizons[0].Name != jazmem.LongTermFile || status.Horizons[0].MaxChars != jazmem.LongTermMaxChars {
		t.Fatalf("unexpected horizons %#v", status.Horizons)
	}
	if len(status.Tasks) != 6 {
		t.Fatalf("expected all scheduler tasks, got %#v", status.Tasks)
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
