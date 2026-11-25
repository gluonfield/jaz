package sessionoverview

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

type overviewStore struct {
	view storage.SessionOverview
}

func TestDisconnectedAgentTasksPreserveNativeCapabilities(t *testing.T) {
	store := overviewStore{view: storage.SessionOverview{
		SessionID:   "parent",
		AgentEvents: []sessionevents.Event{{Type: sessionevents.TypeAgentTask, AgentTask: &sessionevents.AgentTask{ID: "task", State: "running", CanStop: true}}},
	}}
	view, err := NewService(store, overviewLive{}, plugins.New(runtimefiles.New(t.TempDir()))).Load(t.Context(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	if task := view.AgentEvents[0].AgentTask; !task.ConnectionLost || !task.CanStop || task.State != "running" {
		t.Fatalf("disconnected native task = %#v", task)
	}
	if store.view.AgentEvents[0].AgentTask.ConnectionLost {
		t.Fatal("overview changed the stored task")
	}
}

func (s overviewStore) LoadSessionOverview(context.Context, string) (storage.SessionOverview, error) {
	return s.view, nil
}

type overviewLive map[string]acp.HydrationView

func (l overviewLive) HydrationJobs(ids []string) map[string]acp.HydrationView {
	out := make(map[string]acp.HydrationView)
	for _, id := range ids {
		if view := l[id]; view.ID != "" {
			out[id] = view
		}
	}
	return out
}

func TestLoadFoldsLegacySubagentsAndOverlaysLiveThreads(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	session := storage.OverviewThread{
		ID: "child", Slug: "child", Archived: true, ACPAgent: "codex", UpdatedAt: now,
	}
	events := []sessionevents.Event{
		{
			Type: sessionevents.TypeProviderSubagent, At: now,
			ProviderSubagent: &sessionevents.ProviderSubagentEvent{
				Provider: "codex", ID: "/root/newton", Name: "Newton", Task: "Audit", Status: "running",
			},
		},
		{
			Type: sessionevents.TypeProviderSubagent, At: now.Add(-time.Minute),
			ProviderSubagent: &sessionevents.ProviderSubagentEvent{
				Provider: "codex", ID: "/root/newton", Status: "completed",
			},
		},
	}
	service := NewService(
		overviewStore{view: storage.SessionOverview{Threads: []storage.OverviewThread{session}, SubagentEvents: events}},
		overviewLive{"child": {ID: "child", ACPAgent: "CODEX", State: acp.StateRunning, UpdatedAt: now.Add(time.Hour)}},
		plugins.New(runtimefiles.New(t.TempDir())),
	)

	view, err := service.Load(t.Context(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Threads) != 1 || view.Threads[0].State != acp.StateRunning || view.Threads[0].Agent != "codex" || !view.Threads[0].Archived {
		t.Fatalf("threads = %#v", view.Threads)
	}
	if len(view.Subagents) != 1 {
		t.Fatalf("subagents = %#v", view.Subagents)
	}
	subagent := view.Subagents[0]
	if subagent.Name != "Newton" || subagent.Task != "Audit" || subagent.Status != "completed" {
		t.Fatalf("subagent = %#v", subagent)
	}
	if !subagent.UpdatedAt.Equal(now) {
		t.Fatalf("subagent updated at = %s, want %s", subagent.UpdatedAt, now)
	}
}

func TestLoadRefreshesSavedViewsFromInstalledHTML(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store, err := sqlite.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	thread, err := store.CreateSession(storage.CreateSession{Slug: "saved-views"})
	if err != nil {
		t.Fatal(err)
	}
	saved := []sessionevents.PluginView{
		{Reference: "example/status", Title: "Status", File: "status.json", WidgetCode: "<p>Version 1</p>"},
		{Reference: "blocked/status", Title: "Blocked", File: "status.json", WidgetCode: "<p>Blocked snapshot</p>"},
		{Reference: "example/retired", Title: "Retired", File: "status.json", WidgetCode: "<p>Saved retired view</p>"},
		{Reference: "removed/status", Title: "Removed", File: "status.json", WidgetCode: "<p>Saved removed view</p>"},
		{Title: "Standalone", File: "status.json", WidgetCode: "<p>Standalone widget</p>"},
	}
	for i := len(saved) - 1; i >= 0; i-- {
		if err := store.AppendSessionEvents(thread.ID, sessionevents.Event{
			Type: sessionevents.TypeArtifact,
			Artifact: &sessionevents.PresentationEvent{
				Kind: sessionevents.PresentationPluginView, View: &saved[i],
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(layout.Plugins, "example", "views", "status.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	outsideViews := filepath.Join(layout.Root, "views")
	if err := os.Mkdir(outsideViews, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideViews, "status.html")
	if err := os.WriteFile(outside, []byte("<p>Outside store</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(layout.Root, filepath.Join(layout.Plugins, "blocked")); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, overviewLive{}, plugins.New(layout))
	want := slices.Clone(saved)
	want[1].WidgetCode = ""
	check := func() {
		t.Helper()
		view, err := service.Load(t.Context(), thread.ID)
		if err != nil || !slices.Equal(view.PluginViews, want) {
			t.Fatalf("overview views = %+v, %v; want %+v", view.PluginViews, err, want)
		}
	}
	for _, html := range []string{"<p>Version 1</p>", "<p>Version 2</p>"} {
		if err := os.WriteFile(path, []byte(html), 0o600); err != nil {
			t.Fatal(err)
		}
		want[0].WidgetCode = html
		check()
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	want[0].WidgetCode = ""
	check()
	records, err := store.LoadSessionOverview(t.Context(), thread.ID)
	if actual := sessionevents.LatestPluginViews(records.PresentationEvents); err != nil || !slices.Equal(actual, saved) {
		t.Fatalf("saved views = %+v, %v; want %+v", actual, err, saved)
	}
}
