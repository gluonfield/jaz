package acp_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerLeavesGrokModesUnmanagedWhenAgentReportsNoModes(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"grok": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":    "1",
					"JAZ_FAKE_ACP_NO_MODES": "1",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "grok", Slug: "grok-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	status, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Modes.PlanModeID != "" || status.Modes.CurrentModeID != "" || len(status.Modes.AvailableModes) != 0 {
		t.Fatalf("unexpected grok modes %#v", status.Modes)
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ModelProvider != "grok" || session.Model != "" || session.ReasoningEffort != "" {
		t.Fatalf("unexpected session metadata %#v", session)
	}
}

func TestManagerFailsGrokModelOverrideWhenItCannotApplyStartupArgs(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"grok": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Model:   modelcatalog.DefaultGrokModel,
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "grok", Slug: "grok-model"})
	if err == nil || !strings.Contains(err.Error(), "requires the local grok command") {
		t.Fatalf("error = %v", err)
	}
}

func TestManagerRebuildsPromptExtensionsWhenResumingGrokLoopRun(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	marker := "RESUMED_LOOP_WIDGET_PROMPT"
	env := map[string]string{
		"JAZ_FAKE_ACP_AGENT":          "1",
		"JAZ_FAKE_ACP_LOAD":           "1",
		"JAZ_FAKE_ACP_RULES_CONTAINS": marker,
	}
	manager := func() *acp.Manager {
		return acp.NewManager(store, acp.Config{
			Root:      root,
			Workspace: t.TempDir(),
			ResumePrompt: func(session storage.Session) (promptmodule.Modules, error) {
				if session.SourceType != storage.SourceLoopRun || session.SourceID != "run-1" {
					return nil, nil
				}
				return promptmodule.New(marker), nil
			},
			Agents: map[string]acp.AgentConfig{
				acp.AgentGrok: {
					Command: os.Args[0],
					Args:    []string{"-test.run=TestFakeACPAgentProcess"},
					Env:     env,
				},
			},
		}, log.New(io.Discard))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first := manager()
	spawned, err := first.Spawn(ctx, acp.SpawnRequest{
		ACPAgent:               acp.AgentGrok,
		Slug:                   "grok-loop-run",
		SourceType:             storage.SourceLoopRun,
		SourceID:               "run-1",
		ArtifactSurface:        "widget",
		SystemPromptExtensions: promptmodule.New(marker),
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	second := manager()
	if _, err := second.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "after restart", Completion: acp.CompletionInline}); err != nil {
		t.Fatalf("send after restart: %v", err)
	}
	if _, err := second.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = second.Cancel(context.Background(), spawned.SessionID) }()
}
