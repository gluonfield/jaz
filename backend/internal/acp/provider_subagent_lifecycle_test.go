package acp_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloseInterruptsRunningProviderSubagents(t *testing.T) {
	store := newProviderSubagentStore(t)
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentClaude, nil, "", "")
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "subagents-close"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, "subagent toolu_bg=running subagent toolu_done=running subagent toolu_done=completed")
	wantSubagentStatuses(t, store, spawned.SessionID, map[string]string{"toolu_bg": "running", "toolu_done": "completed"})
	manager.Close()
	wantSubagentStatuses(t, store, spawned.SessionID, map[string]string{"toolu_bg": "cancelled", "toolu_done": "completed"})
}

func TestResumeAfterCrashInterruptsSubagentsUntilReportedAgain(t *testing.T) {
	store := newProviderSubagentStore(t)
	root := t.TempDir()
	crashed := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, nil, "", "")
	t.Cleanup(crashed.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	spawned, err := crashed.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "subagents-crash"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, crashed, spawned.SessionID, "subagent toolu_bg=running subagent toolu_done=running subagent toolu_done=completed")

	restarted := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, nil, "", "")
	t.Cleanup(restarted.Close)
	sendAndWait(t, ctx, restarted, spawned.SessionID, "after restart")
	wantSubagentStatuses(t, store, spawned.SessionID, map[string]string{"toolu_bg": "cancelled", "toolu_done": "completed"})
	sendAndWait(t, ctx, restarted, spawned.SessionID, "subagent toolu_bg=running")
	wantSubagentStatuses(t, store, spawned.SessionID, map[string]string{"toolu_bg": "running", "toolu_done": "completed"})
}

func newProviderSubagentStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func wantSubagentStatuses(t *testing.T, store *sqlitestore.Store, sessionID string, want map[string]string) {
	t.Helper()
	view, err := store.LoadSessionOverview(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, event := range view.SubagentEvents {
		got[event.ProviderSubagent.ID] = event.ProviderSubagent.Status
	}
	if !maps.Equal(got, want) {
		t.Fatalf("subagent statuses = %v, want %v", got, want)
	}
}
