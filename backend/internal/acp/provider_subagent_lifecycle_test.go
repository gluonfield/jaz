package acp_test

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionoverview"
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

// Codex and Claude, and rows older Jaz stored, name a subagent's state in many
// words. Each reads as running or as one way of ending, so the Subagents list
// and the end of the agent's process agree on which are still at work.
func TestEveryProviderSubagentStateReadsAsRunningOrEnded(t *testing.T) {
	const (
		running   = sessionevents.SubagentRunning
		completed = sessionevents.SubagentCompleted
		failed    = sessionevents.SubagentFailed
		cancelled = sessionevents.SubagentCancelled
	)
	sent := map[string]sessionevents.ProviderSubagentStatus{
		"running": running, "starting": running, "pending_init": running,
		"completed": completed, "done": completed,
		"failed": failed, "error": failed, "errored": failed, "not_found": failed,
		"cancelled": cancelled, "canceled": cancelled, "interrupted": cancelled,
		"stopped": cancelled, "shutdown": cancelled, "closed": cancelled,
	}
	store := newProviderSubagentStore(t)
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentClaude, nil, "", "")
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "subagent-states"})
	if err != nil {
		t.Fatal(err)
	}
	// Ended subagents report last, so only the status can list running ones
	// first.
	var reports []string
	for word, status := range sent {
		report := "subagent " + word + "=" + word
		if status == running {
			reports = append([]string{report}, reports...)
		} else {
			reports = append(reports, report)
		}
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, strings.Join(reports, " "))

	view, err := sessionoverview.NewService(store, manager).Load(ctx, spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]sessionevents.ProviderSubagentStatus{}
	for i, subagent := range view.Subagents {
		listed[subagent.ID] = subagent.Status
		if i > 0 && sent[view.Subagents[i-1].ID] != running && sent[subagent.ID] == running {
			t.Fatalf("%s is listed after the ended %s", subagent.ID, view.Subagents[i-1].ID)
		}
	}
	if !maps.Equal(listed, sent) {
		t.Fatalf("listed statuses = %v, want %v", listed, sent)
	}
	manager.Close()
	closed := map[string]string{}
	for word, status := range sent {
		if status == running {
			status = cancelled
		}
		closed[word] = string(status)
	}
	wantSubagentStatuses(t, store, spawned.SessionID, closed)
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
		got[event.ProviderSubagent.ID] = string(event.ProviderSubagent.Status)
	}
	if !maps.Equal(got, want) {
		t.Fatalf("subagent statuses = %v, want %v", got, want)
	}
}
