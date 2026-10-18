package acp_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func newTwoAgentManager(t *testing.T, store acp.Store, workspace string) *acp.Manager {
	t.Helper()
	agent := func() acp.AgentConfig {
		return acp.AgentConfig{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeACPAgentProcess"},
			Env: map[string]string{
				"JAZ_FAKE_ACP_AGENT":                "1",
				"JAZ_FAKE_ACP_LOAD":                 "1",
				"JAZ_FAKE_ACP_MATERIALIZED_SESSION": filepath.Join(t.TempDir(), "session"),
			},
		}
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: workspace,
		Agents:    map[string]acp.AgentConfig{acp.AgentCodex: agent(), acp.AgentClaude: agent()},
	}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	return manager
}

func TestSwitchAgentStartsAFreshNativeSessionThatSurvivesARestart(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	manager := newTwoAgentManager(t, store, workspace)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "switching-bot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendUserMessage(store, spawned.SessionID, "prior turn", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(spawned.SessionID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "prior reply"}); err != nil {
		t.Fatal(err)
	}

	if err := manager.SwitchAgent(ctx, spawned.SessionID, acp.AgentClaude); err != nil {
		t.Fatal(err)
	}
	switched, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if switched.RuntimeRef.Agent != acp.AgentClaude || switched.RuntimeRef.SessionID != "" {
		t.Fatalf("switched session = agent %q, native session %q", switched.RuntimeRef.Agent, switched.RuntimeRef.SessionID)
	}

	manager.Close()
	restarted := newTwoAgentManager(t, store, workspace)
	job, err := restarted.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "hello", Completion: acp.CompletionInline})
	if err != nil {
		t.Fatalf("first turn after the switch: %v", err)
	}
	if job.ACPAgent != acp.AgentClaude {
		t.Fatalf("turn ran on %q", job.ACPAgent)
	}
	if done, err := restarted.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil || done.State != acp.StateIdle {
		t.Fatalf("first turn after the switch = %#v, %v", done, err)
	}
	resumed, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RuntimeRef.SessionID == "" {
		t.Fatal("the new agent's native session was not kept after its first prompt")
	}
}
