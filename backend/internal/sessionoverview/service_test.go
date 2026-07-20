package sessionoverview

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type overviewStore struct {
	view storage.SessionOverview
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
