package acp_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerRestoresClaudeBaselineModeAfterPlan(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"claude": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":        "1",
					"JAZ_FAKE_ACP_CLAUDE_MODES": "1",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "claude", Slug: "claude-force-mode"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	status, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Modes.CurrentModeID != "bypassPermissions" {
		t.Fatalf("spawn modes = %#v, want bypassPermissions baseline", status.Modes)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "make a plan", Completion: acp.CompletionInline, PlanRequested: true}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "plan" {
		t.Fatalf("plan mode = %q, want plan", job.Modes.CurrentModeID)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "approved", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "bypassPermissions" {
		t.Fatalf("mode after plan = %q, want bypassPermissions", job.Modes.CurrentModeID)
	}
}

func TestManagerLeavesLoadedPlanModeBeforeOrdinarySend(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "loaded-plan-mode",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "claude",
			SessionID: "fake-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"claude": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":        "1",
					"JAZ_FAKE_ACP_LOAD":         "1",
					"JAZ_FAKE_ACP_CLAUDE_MODES": "1",
					"JAZ_FAKE_ACP_CURRENT_MODE": "plan",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := manager.Send(ctx, acp.SendRequest{Session: session.ID, Message: "approved", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "bypassPermissions" {
		t.Fatalf("modes after ordinary send = %#v, want bypassPermissions baseline", job.Modes)
	}
	if job.Assistant != "hello from fake agent" || len(job.Plan) != 0 {
		t.Fatalf("ordinary send stayed in plan mode: assistant=%q plan=%#v", job.Assistant, job.Plan)
	}
}

func TestManagerForcesBaselineWhenLoadedModeLooksAlreadyRestored(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "loaded-stale-plan-mode",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "fake-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"codex": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":         "1",
					"JAZ_FAKE_ACP_LOAD":          "1",
					"JAZ_FAKE_ACP_CURRENT_MODE":  "plan",
					"JAZ_FAKE_ACP_REPORTED_MODE": "full-access",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := manager.Send(ctx, acp.SendRequest{Session: session.ID, Message: "approved", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "full-access" {
		t.Fatalf("modes after ordinary send = %#v, want full-access baseline", job.Modes)
	}
	if job.Assistant != "hello from fake agent" || len(job.Plan) != 0 {
		t.Fatalf("ordinary send stayed in stale plan mode: assistant=%q plan=%#v", job.Assistant, job.Plan)
	}
}
