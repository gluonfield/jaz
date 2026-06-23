package browsertask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browserworker"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeManager struct {
	statusRef string
	statusJob acp.Job
	statusErr error
	spawn     acp.SpawnRequest
	send      acp.SendRequest
	wait      acp.WaitRequest
	cancel    string
	job       acp.Job
}

type fakeExtension struct {
	connected bool
}

func (f fakeExtension) Status() browserworker.ExtensionStatus {
	return browserworker.ExtensionStatus{Connected: f.connected}
}

func (f *fakeManager) Status(ref string) (acp.Job, error) {
	f.statusRef = ref
	if f.statusErr != nil {
		return acp.Job{}, f.statusErr
	}
	return f.statusJob, nil
}

func (f *fakeManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.spawn = req
	return acp.SpawnResult{SessionID: "browser-session", Slug: req.Slug, ACPAgent: req.ACPAgent, State: acp.StateIdle}, nil
}

func (f *fakeManager) Send(_ context.Context, req acp.SendRequest) (acp.Job, error) {
	f.send = req
	return acp.Job{State: acp.StateRunning}, nil
}

func (f *fakeManager) Wait(_ context.Context, req acp.WaitRequest) (acp.Job, error) {
	f.wait = req
	return f.job, nil
}

func (f *fakeManager) Cancel(_ context.Context, session string) (acp.Job, error) {
	f.cancel = session
	return acp.Job{}, nil
}

func TestBrowserTaskSpawnsRestrictedWorker(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentCodex)
	manager := &fakeManager{
		statusErr: errors.New("not found"),
		job:       acp.Job{ID: "browser-session", Slug: "browser-red-rooster", State: acp.StateIdle, Assistant: "Done."},
	}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{connected: true})

	out, err := service.Run(context.Background(), Request{
		Kind:       KindGet,
		Task:       "Find the company page.",
		URL:        "https://www.linkedin.com/company/jaz",
		SessionKey: "Red Rooster!",
		ParentID:   "parent-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer != "Done." || out.SessionKey != "red-rooster" {
		t.Fatalf("result = %#v", out)
	}
	wantSlug := workerSlug("parent-session", "red-rooster")
	if manager.statusRef != wantSlug {
		t.Fatalf("status ref = %q, want %q", manager.statusRef, wantSlug)
	}
	if manager.spawn.ParentID != "parent-session" || manager.spawn.ACPAgent != acp.AgentCodex {
		t.Fatalf("spawn parent/agent = %#v", manager.spawn)
	}
	if manager.spawn.Slug != wantSlug || manager.spawn.Title != "Browser: red-rooster" {
		t.Fatalf("spawn title/slug = %q/%q", manager.spawn.Title, manager.spawn.Slug)
	}
	if manager.spawn.SourceType != storage.SourceBrowserTask || manager.spawn.SourceID != sourceID("parent-session", "red-rooster") {
		t.Fatalf("source = %q/%q", manager.spawn.SourceType, manager.spawn.SourceID)
	}
	workerPrompt := manager.spawn.SystemPromptExtensions.Text()
	for _, want := range []string{"browser worker", "real filters/facets", "paginate"} {
		if !strings.Contains(workerPrompt, want) {
			t.Fatalf("worker prompt missing %q:\n%s", want, workerPrompt)
		}
	}
	if manager.spawn.Directory != workerDir {
		t.Fatalf("directory = %q", manager.spawn.Directory)
	}
	if manager.spawn.Model != "gpt-5.4-mini" || manager.spawn.ReasoningEffort != "low" {
		t.Fatalf("model/effort = %q/%q", manager.spawn.Model, manager.spawn.ReasoningEffort)
	}
	if manager.send.Session != "browser-session" || manager.send.Completion != acp.CompletionInline || manager.send.ParentVisible {
		t.Fatalf("send = %#v", manager.send)
	}
	for _, want := range []string{
		"Task kind: get",
		"Starting URL: https://www.linkedin.com/company/jaz",
		".jaz-runtime/browser/website-notes/www-linkedin-com.md",
		"Find the company page.",
		"compact final answer",
	} {
		if !strings.Contains(manager.send.Message, want) {
			t.Fatalf("prompt missing %q:\n%s", want, manager.send.Message)
		}
	}
	if manager.wait.Session != "browser-session" || manager.wait.Timeout != Timeout {
		t.Fatalf("wait = %#v", manager.wait)
	}
}

func TestBrowserTaskReusesIdleKeyedWorker(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentClaude)
	manager := &fakeManager{
		statusJob: acp.Job{ID: "existing-session", Slug: workerSlug("parent-session", defaultKey), State: acp.StateIdle},
		job:       acp.Job{ID: "existing-session", Slug: "browser-default", State: acp.StateIdle, Assistant: "Still signed in."},
	}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{connected: true})

	if _, err := service.Run(context.Background(), Request{Kind: KindCheck, Task: "Am I logged in?", ParentID: "parent-session"}); err != nil {
		t.Fatal(err)
	}
	if manager.spawn.ACPAgent != "" {
		t.Fatalf("spawned unexpectedly: %#v", manager.spawn)
	}
	if manager.send.Session != "existing-session" {
		t.Fatalf("send session = %q", manager.send.Session)
	}
}

func TestBrowserTaskReturnsBusyForRunningKeyedWorker(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentCodex)
	manager := &fakeManager{statusJob: acp.Job{ID: "existing-session", State: acp.StateRunning}}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{connected: true})

	_, err := service.Run(context.Background(), Request{Kind: KindDo, Task: "Open LinkedIn", SessionKey: "linkedin", ParentID: "parent-session"})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v", err)
	}
	if manager.send.Session != "" {
		t.Fatalf("sent unexpectedly: %#v", manager.send)
	}
}

func TestBrowserTaskRejectsDisabledSettings(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveBrowserSettings(store, jazsettings.BrowserSettings{Enabled: false, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{connected: true})

	_, err := service.Run(context.Background(), Request{Kind: KindDo, Task: "Open a page", ParentID: "parent-session"})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("err = %v", err)
	}
}

func TestBrowserTaskToolsEnabledDoesNotDependOnExtensionConnection(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentCodex)
	service := New(store, &fakeManager{}, acp.BuiltinAgents(), fakeExtension{})

	if !service.MCPToolsEnabled() {
		t.Fatal("browser task tools should be advertised when settings are enabled")
	}
}

func TestBrowserTaskRejectsDisconnectedExtension(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentCodex)
	manager := &fakeManager{}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{})

	_, err := service.Run(context.Background(), Request{Kind: KindDo, Task: "Open a page", ParentID: "parent-session"})
	if err == nil || !strings.Contains(err.Error(), "Chrome extension") {
		t.Fatalf("err = %v", err)
	}
	if manager.send.Session != "" {
		t.Fatalf("sent unexpectedly: %#v", manager.send)
	}
}

func TestBrowserTaskTimeoutUsesOverride(t *testing.T) {
	store := newStore(t)
	saveBrowserDefaults(t, store, acp.AgentCodex)
	manager := &fakeManager{
		statusErr: errors.New("not found"),
		job:       acp.Job{ID: "browser-session", State: acp.StateRunning},
	}
	service := New(store, manager, acp.BuiltinAgents(), fakeExtension{connected: true})
	service.Timeout = time.Second

	_, err := service.Run(context.Background(), Request{Kind: KindDo, Task: "Slow task", ParentID: "parent-session"})
	if err == nil || !strings.Contains(err.Error(), "1s") {
		t.Fatalf("err = %v", err)
	}
	if manager.wait.Timeout != time.Second {
		t.Fatalf("timeout = %s", manager.wait.Timeout)
	}
	if manager.cancel != "browser-session" {
		t.Fatalf("cancel = %q", manager.cancel)
	}
}

func newStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func saveBrowserDefaults(t *testing.T, store *sqlitestore.Store, agent string) {
	t.Helper()
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		agent: {Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveBrowserSettings(store, jazsettings.BrowserSettings{Enabled: true, Agent: agent}); err != nil {
		t.Fatal(err)
	}
}
