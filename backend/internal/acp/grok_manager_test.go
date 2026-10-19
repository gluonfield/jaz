package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
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
	defer manager.Close()
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

func TestManagerStartsGrokModelWithRulesAndSetsAdvertisedEffort(t *testing.T) {
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	manager, store := newGrokOptionsManager(t, "grok-4.6", map[string]string{"JAZ_FAKE_ACP_REQUEST_LOG": requestLog})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "grok", Slug: "grok-model", SystemPromptExtensions: promptmodule.New("grok rules marker")})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	raw, err := os.ReadFile(requestLog)
	if err != nil {
		t.Fatal(err)
	}
	var modelID string
	var configIDs []string
	sessions := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var request struct {
			Method string `json:"method"`
			Params struct {
				Meta     map[string]any `json:"_meta"`
				ConfigID string         `json:"configId"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "session/new":
			sessions++
			if rules, _ := request.Params.Meta["rules"].(string); strings.Contains(rules, "grok rules marker") {
				modelID, _ = request.Params.Meta["modelId"].(string)
			}
		case "session/set_config_option":
			configIDs = append(configIDs, request.Params.ConfigID)
		case "session/set_model":
			t.Fatal("grok model must not change after session/new")
		}
	}
	if sessions != 1 || modelID != "grok-4.6" || strings.Join(configIDs, ",") != "reasoning_effort" {
		t.Fatalf("session/new count = %d, modelId = %q, set_config_option ids = %v", sessions, modelID, configIDs)
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ReasoningEffort != "xhigh" {
		t.Fatalf("session reasoning effort = %q", session.ReasoningEffort)
	}
}

func TestManagerRejectsGrokModelItDoesNotAdvertise(t *testing.T) {
	manager, _ := newGrokOptionsManager(t, "grok-9", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "grok", Slug: "grok-model"})
	if err == nil || !strings.Contains(err.Error(), "not advertised") {
		t.Fatalf("spawn error = %v", err)
	}
}

func TestManagerJudgesGrokModelAgainstListAnnouncedAfterLoginRenewal(t *testing.T) {
	for model, wantErr := range map[string]string{"grok-4.7": "", "grok-9": "not advertised"} {
		t.Run(model, func(t *testing.T) {
			manager, _ := newGrokOptionsManager(t, model, map[string]string{
				"JAZ_FAKE_ACP_MODELS":      "grok-4.5,grok-4.6",
				"JAZ_FAKE_ACP_LATE_MODELS": "grok-4.7,grok-4.6,grok-4.5",
			})
			defer manager.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			started := time.Now()
			_, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentGrok, Slug: "grok-late-models"})
			if waited := time.Since(started); waited > 3*time.Second {
				t.Fatalf("start waited %s instead of ending at the announcement", waited)
			}
			if (err == nil) != (wantErr == "") || (err != nil && !strings.Contains(err.Error(), wantErr)) {
				t.Fatalf("spawn error = %v, want %q", err, wantErr)
			}
		})
	}
}

func TestManagerRefusesGrokModelBeforeSessionWhenNoListIsAnnounced(t *testing.T) {
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	manager, _ := newGrokOptionsManager(t, "grok-4.7", map[string]string{
		"JAZ_FAKE_ACP_MODELS":            "grok-4.5,grok-4.6",
		"JAZ_FAKE_ACP_LATE_MODELS":       "grok-4.7,grok-4.6,grok-4.5",
		"JAZ_FAKE_ACP_LATE_MODELS_AFTER": "1m",
		"JAZ_FAKE_ACP_REQUEST_LOG":       requestLog,
	})
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentGrok, Slug: "grok-no-models"})
	if err == nil || !strings.Contains(err.Error(), "did not announce its model list") {
		t.Fatalf("spawn error = %v", err)
	}
	raw, err := os.ReadFile(requestLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"method":"session/new"`) {
		t.Fatal("session/new ran against the provisional model list")
	}
}

func newGrokOptionsManager(t *testing.T, model string, extraEnv map[string]string) (*acp.Manager, *jsonstore.Store) {
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"JAZ_FAKE_ACP_AGENT":         "1",
		"JAZ_FAKE_ACP_SET_CONFIG":    "1",
		"JAZ_FAKE_ACP_EXPECT_EFFORT": "xhigh",
		"JAZ_FAKE_ACP_SESSION_OPTIONS": `[
			{"id":"model","category":"model","type":"select","currentValue":"grok-4.7","options":[{"value":"grok-4.7","name":"Grok 4.7"},{"value":"grok-4.6","name":"Grok 4.6"}]},
			{"id":"reasoning_effort","category":"thought_level","type":"select","currentValue":"high","options":[{"value":"xhigh","name":"Extra High"},{"value":"high","name":"High"}]}
		]`,
	}
	maps.Copy(env, extraEnv)
	return acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"grok": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           model,
				ReasoningEffort: "xhigh",
				Env:             env,
			},
		},
	}, log.New(io.Discard)), store
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
	defer second.Close()
}
