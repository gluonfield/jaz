package acp_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// staticPrompt is a fixed acp.SystemPromptSource for tests.
type staticPrompt string

func (s staticPrompt) SkillsPrompt() string { return string(s) }

func TestManagerSpawnsFakeACPAgentAndStoresSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeNative})
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:         t.TempDir(),
		Workspace:    t.TempDir(),
		SystemPrompt: staticPrompt("skill prompt"),
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "fake-large",
				ReasoningEffort: "high",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":                "1",
					"JAZ_FAKE_ACP_EXPECT_TERMINAL_AUTH": "1",
					"JAZ_FAKE_ACP_SYSTEM_PROMPT":        "skill prompt",
					"JAZ_FAKE_ACP_SET_MODEL":            "1",
					"JAZ_FAKE_ACP_EXPECT_MODEL":         "fake-large",
					"JAZ_FAKE_ACP_SET_CONFIG":           "1",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":        "high",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{
		ParentID: parent.ID,
		ACPAgent: "fake",
		Slug:     "fake-review",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if spawned.State != acp.StateIdle {
		t.Fatalf("spawn state = %s, want %s", spawned.State, acp.StateIdle)
	}
	status, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Modes.PlanModeID != "plan" || status.Modes.ExecutionModeID != "full-access" {
		t.Fatalf("unexpected modes %#v", status.Modes)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("spawn should not store task messages: %#v", messages)
	}

	done := make(chan acp.Job, 2)
	manager.Done = func(_ context.Context, job acp.Job) { done <- job }

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "say hello", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle {
		t.Fatalf("state = %s, want %s; error=%s", job.State, acp.StateIdle, job.Error)
	}
	if job.Assistant != "hello from fake agent" {
		t.Fatalf("assistant = %q", job.Assistant)
	}

	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ParentID != parent.ID || session.Runtime != storage.RuntimeACP || session.RuntimeRef.SessionID != "fake-session" {
		t.Fatalf("unexpected session metadata %#v", session)
	}
	if session.ModelProvider != "fake" || session.Model != "fake-large" || session.ReasoningEffort != "high" {
		t.Fatalf("unexpected session model metadata %#v", session)
	}
	messages, err = store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || provider.MessageContent(messages[0]) != "say hello" {
		t.Fatalf("unexpected messages %#v", messages)
	}
	events, err := store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasACPMessage(events, "hello from fake agent") || !hasACPTool(events, "whoami") {
		t.Fatalf("missing ACP transcript events %#v", events)
	}
	select {
	case job := <-done:
		t.Fatalf("sync task propagated async completion: %#v", job)
	case <-time.After(100 * time.Millisecond):
	}
	activity, err := store.LoadActivity(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 1 || activity[0].Text != "whoami" || activity[0].Status != "completed" {
		t.Fatalf("unexpected activity %#v", activity)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "again", Completion: acp.CompletionAsync, ParentVisible: true}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Assistant != "hello from fake agent" {
		t.Fatalf("assistant after follow-up = %q", job.Assistant)
	}
	messages, err = store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || provider.MessageContent(messages[1]) != "again" {
		t.Fatalf("unexpected follow-up messages %#v", messages)
	}
	events, err = store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if countACPMessage(events, "hello from fake agent") < 2 {
		t.Fatalf("missing follow-up ACP transcript event %#v", events)
	}
	parentEvents, err := store.LoadSessionEvents(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasACPMessage(parentEvents, "hello from fake agent") || hasACPTool(parentEvents, "whoami") {
		t.Fatalf("parent leaked child transcript details %#v", parentEvents)
	}
	if !hasACPStatus(parentEvents, spawned.SessionID) {
		t.Fatalf("parent missing child status surface %#v", parentEvents)
	}
	select {
	case job := <-done:
		if job.ID != spawned.SessionID {
			t.Fatalf("unexpected propagated job %#v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("async task did not propagate completion")
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "make a plan", Completion: acp.CompletionInline, PlanRequested: true}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Plan) != 2 || job.Plan[0].Status != "completed" || job.Plan[1].Status != "in_progress" {
		t.Fatalf("unexpected plan %#v", job.Plan)
	}
	if job.Modes.CurrentModeID != "plan" {
		t.Fatalf("current mode = %q, want plan", job.Modes.CurrentModeID)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.Slug, Message: "approved", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.Modes.CurrentModeID != "full-access" {
		t.Fatalf("current mode after approval = %q, want full-access", job.Modes.CurrentModeID)
	}
}

func TestManagerUsesStoredACPCommandArgs(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{
		ACP: map[string]agentsettings.ACPAgentDefaults{
			"codex": {
				Enabled:         true,
				Command:         agentsettings.CommandLine(os.Args[0], []string{"-test.run=TestFakeACPAgentProcess"}),
				Model:           "fake-large",
				ReasoningEffort: "high",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog := acp.AgentCatalog{
		"codex": {
			Command: "missing-codex-acp",
			Env: map[string]string{
				"JAZ_FAKE_ACP_AGENT":         "1",
				"JAZ_FAKE_ACP_SET_MODEL":     "1",
				"JAZ_FAKE_ACP_EXPECT_MODEL":  "fake-large/high",
				"JAZ_FAKE_ACP_SET_CONFIG":    "1",
				"JAZ_FAKE_ACP_EXPECT_EFFORT": "high",
			},
		},
	}
	manager := acp.NewManager(store, acp.Config{
		Root:        t.TempDir(),
		Workspace:   t.TempDir(),
		AgentSource: agentsettings.NewACPConfigSource(store, catalog),
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "stored-command"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Model != "fake-large" || session.ReasoningEffort != "high" {
		t.Fatalf("unexpected session model metadata %#v", session)
	}
}

func TestManagerDoesNotFallbackWhenAgentSettingsAreCorrupt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveSetting(agentsettings.AgentSettingsNamespace, agentsettings.AgentDefaultsKey, []byte(`{"acp":"bad"}`)); err != nil {
		t.Fatal(err)
	}
	catalog := acp.AgentCatalog{
		"codex": {
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeACPAgentProcess"},
			Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
		},
	}
	manager := acp.NewManager(store, acp.Config{
		Root:        t.TempDir(),
		Workspace:   t.TempDir(),
		AgentSource: agentsettings.NewACPConfigSource(store, catalog),
		Agents:      catalog,
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "corrupt-settings"})
	if err == nil {
		t.Fatal("expected corrupt settings to block spawn")
	}
	if _, loadErr := store.LoadSession("corrupt-settings"); loadErr == nil {
		t.Fatal("spawn should fail before creating a session")
	}
}

func TestManagerFailsWhenConfiguredModelIsUnsupported(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManagerWithModel(t, store, t.TempDir(), nil, "fake-large")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "unsupported-model"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), "session/set_model is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
	session, loadErr := store.LoadSession("unsupported-model")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if session.Status != storage.StatusError || !strings.Contains(session.Error, "session/set_model is not supported") {
		t.Fatalf("unsupported model failure was not stored: %#v", session)
	}
}

func TestManagerFailsWhenConfiguredReasoningEffortIsUnsupported(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManagerWithOptions(t, store, t.TempDir(), nil, "", "high")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "unsupported-effort"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), "session/set_config_option is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
	session, loadErr := store.LoadSession("unsupported-effort")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if session.Status != storage.StatusError || !strings.Contains(session.Error, "session/set_config_option is not supported") {
		t.Fatalf("unsupported reasoning effort failure was not stored: %#v", session)
	}
}

func TestManagerUsesClaudeEffortConfigOption(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"claude": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "default",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "default,sonnet",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "default",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":    "effort",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "xhigh",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "claude", Slug: "claude-effort"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ModelProvider != "claude" || session.Model != "default" || session.ReasoningEffort != "xhigh" {
		t.Fatalf("unexpected session metadata %#v", session)
	}
}

func TestManagerCanonicalizesClaudeAliasBeforeSettingModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := "claude-fable-5[1m]"
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"claude": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           model,
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "default," + model,
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": model,
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":    "effort",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "xhigh",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	legacyClaudeName := strings.ReplaceAll("claude-code", "-", "_")
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: legacyClaudeName, Slug: "claude-fable"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if spawned.ACPAgent != "claude" || session.ModelProvider != "claude" || session.RuntimeRef.Agent != "claude" || session.Model != model {
		t.Fatalf("unexpected session metadata spawned=%#v session=%#v", spawned, session)
	}
}

func TestManagerRejectsUnsupportedClaudeModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"claude": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "claude-opus-4.8",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":  "1",
					"JAZ_FAKE_ACP_MODELS": "default,sonnet",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "claude", Slug: "bad-claude-model"})
	if err == nil {
		t.Fatal("expected unsupported claude model to fail")
	}
	if !strings.Contains(err.Error(), "available model ids: default, sonnet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagerEncodesCodexReasoningEffortInModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"codex": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "fake-large",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":        "1",
					"JAZ_FAKE_ACP_MODELS":       "fake-large/xhigh",
					"JAZ_FAKE_ACP_SET_MODEL":    "1",
					"JAZ_FAKE_ACP_EXPECT_MODEL": "fake-large/xhigh",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "codex-model-effort"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Model != "fake-large" || session.ReasoningEffort != "xhigh" {
		t.Fatalf("unexpected stored model metadata %#v", session)
	}
}

func TestManagerRejectsUnavailableCodexModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"codex": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "missing-model",
				ReasoningEffort: "medium",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":  "1",
					"JAZ_FAKE_ACP_MODELS": "fake-large/medium",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "missing-codex-model"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), "not advertised") || !strings.Contains(err.Error(), "fake-large") {
		t.Fatalf("unexpected error: %v", err)
	}
	session, loadErr := store.LoadSession("missing-codex-model")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if session.Status != storage.StatusError || !strings.Contains(session.Error, "not advertised") {
		t.Fatalf("unavailable model failure was not stored: %#v", session)
	}
}

func TestManagerRejectsUnavailableCodexReasoningEffort(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"codex": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           "fake-large",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":  "1",
					"JAZ_FAKE_ACP_MODELS": "fake-large/medium",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "missing-codex-effort"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), "fake-large/xhigh") || !strings.Contains(err.Error(), "fake-large/medium") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func hasACPMessage(events []sessionevents.Event, content string) bool {
	return countACPMessage(events, content) > 0
}

func countACPMessage(events []sessionevents.Event, content string) int {
	count := 0
	for _, event := range events {
		if event.Type == "acp_message" && event.Content == content {
			count++
		}
	}
	return count
}

func hasACPTool(events []sessionevents.Event, title string) bool {
	for _, event := range events {
		if event.Type != "acp_tool" || event.ACP == nil {
			continue
		}
		for _, call := range event.ACP.ToolCalls {
			if call.Title == title {
				return true
			}
		}
	}
	return false
}

func hasACPStatus(events []sessionevents.Event, id string) bool {
	for _, event := range events {
		if event.Type == "acp" && event.ACP != nil && event.ACP.ID == id {
			return true
		}
	}
	return false
}

func newFakeAgentManager(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string) *acp.Manager {
	return newFakeAgentManagerWithModel(t, store, root, extraEnv, "")
}

func newFakeAgentManagerWithModel(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string, model string) *acp.Manager {
	return newFakeAgentManagerWithOptions(t, store, root, extraEnv, model, "")
}

func newFakeAgentManagerWithOptions(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string, model, effort string) *acp.Manager {
	env := map[string]string{"JAZ_FAKE_ACP_AGENT": "1"}
	for key, value := range extraEnv {
		env[key] = value
	}
	return acp.NewManager(store, acp.Config{
		Root:      root,
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command:         os.Args[0],
				Args:            []string{"-test.run=TestFakeACPAgentProcess"},
				Model:           model,
				ReasoningEffort: effort,
				Env:             env,
			},
		},
	}, log.New(io.Discard))
}

func TestManagerResumesStoredSessionAfterRestart(t *testing.T) {
	for name, loadSupported := range map[string]bool{"via session/load": true, "via fresh session": false} {
		t.Run(name, func(t *testing.T) {
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			env := map[string]string{
				"JAZ_FAKE_ACP_SET_MODEL":     "1",
				"JAZ_FAKE_ACP_EXPECT_MODEL":  "fake-large",
				"JAZ_FAKE_ACP_SET_CONFIG":    "1",
				"JAZ_FAKE_ACP_EXPECT_EFFORT": "high",
			}
			if loadSupported {
				env["JAZ_FAKE_ACP_LOAD"] = "1"
			}
			first := newFakeAgentManagerWithOptions(t, store, root, env, "fake-large", "high")

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			spawned, err := first.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-resume"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := first.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "say hello", Completion: acp.CompletionInline}); err != nil {
				t.Fatal(err)
			}
			if _, err := first.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil {
				t.Fatal(err)
			}
			first.Close()

			// A new manager (server restart) has no live job for the session;
			// sending must transparently resume it.
			second := newFakeAgentManagerWithOptions(t, store, root, env, "fake-large", "high")
			if _, err := second.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "after restart", Completion: acp.CompletionInline}); err != nil {
				t.Fatalf("send after restart: %v", err)
			}
			job, err := second.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = second.Cancel(context.Background(), spawned.SessionID) }()
			if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
				t.Fatalf("resumed turn state=%s assistant=%q error=%q", job.State, job.Assistant, job.Error)
			}
			if job.ACPSession != "fake-session" {
				t.Fatalf("acp session = %q", job.ACPSession)
			}
			messages, err := store.LoadMessages(spawned.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != 2 || provider.MessageContent(messages[1]) != "after restart" {
				t.Fatalf("unexpected messages %#v", messages)
			}
			events, err := store.LoadSessionEvents(spawned.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			// The load replay must not be re-recorded as new transcript events.
			if countACPMessage(events, "replayed history") != 0 {
				t.Fatalf("history replay leaked into events %#v", events)
			}
		})
	}
}

func TestCancelStopsRunningTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Cancel(ctx, spawned.SessionID); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateCancelled {
		t.Fatalf("state = %s, want cancelled (error=%q)", job.State, job.Error)
	}
	for _, call := range job.ToolCalls {
		if call.Status != "cancelled" {
			t.Fatalf("dangling tool call left as %q: %#v", call.Status, call)
		}
	}
	events, err := store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Type == "acp" && event.ACP != nil && event.ACP.State == acp.StateCancelled {
			found = true
		}
	}
	if !found {
		t.Fatalf("no cancelled status event was published: %#v", events)
	}

	// The graceful path keeps the agent process alive for the next turn.
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "say hello", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err = manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
		t.Fatalf("follow-up turn state=%s assistant=%q error=%q", job.State, job.Assistant, job.Error)
	}
}

func TestInteractiveTextCancelsRunningTurnBeforeManagedSend(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-steer"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if err := manager.AnswerInteractive(ctx, acp.InteractiveAnswer{Session: spawned.SessionID, Text: "say hello"}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
		t.Fatalf("steered turn state=%s assistant=%q error=%q", job.State, job.Assistant, job.Error)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 ||
		provider.MessageContent(messages[0]) != "block until cancelled" ||
		provider.MessageContent(messages[1]) != "say hello" {
		t.Fatalf("unexpected messages after steer %#v", messages)
	}
	events, err := store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasACPMessage(events, "hello from fake agent") {
		t.Fatalf("managed steered turn did not publish assistant transcript: %#v", events)
	}
}

func TestInteractiveTextWaitsForCancelledTurnFinishedBeforeManagedSend(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-steer-finish-order"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	cancelFinishedEntered := make(chan struct{})
	releaseCancelFinished := make(chan struct{})
	released := false
	manager.TurnFinished = func(_ context.Context, job acp.Job) {
		if job.ID == spawned.SessionID && job.State == acp.StateCancelled {
			close(cancelFinishedEntered)
			<-releaseCancelFinished
		}
	}
	defer func() {
		if !released {
			close(releaseCancelFinished)
		}
	}()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	answerDone := make(chan error, 1)
	go func() {
		answerDone <- manager.AnswerInteractive(ctx, acp.InteractiveAnswer{Session: spawned.SessionID, Text: "say hello"})
	}()

	select {
	case <-cancelFinishedEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled turn did not reach TurnFinished")
	}
	select {
	case err := <-answerDone:
		t.Fatalf("AnswerInteractive returned before TurnFinished completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseCancelFinished)
	released = true
	if err := <-answerDone; err != nil {
		t.Fatal(err)
	}
}

func TestSpawnSessionDirectories(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: workspace,
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
			},
		},
	}, log.New(io.Discard))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Explicit directory: created under the workspace and persisted.
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "dir-task", Directory: "ink-backend"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	want := filepath.Join(workspace, "ink-backend")
	if spawned.Cwd != want {
		t.Fatalf("cwd = %q, want %q", spawned.Cwd, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("directory was not created: %v", err)
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Cwd != want {
		t.Fatalf("cwd not persisted: %#v", session.RuntimeRef)
	}

	// No directory: a fresh per-session directory named after the slug.
	spawned, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "adhoc-task"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	if spawned.Cwd != filepath.Join(workspace, spawned.Slug) {
		t.Fatalf("default cwd = %q, want workspace/%s", spawned.Cwd, spawned.Slug)
	}

	// Escapes are rejected and the failure lands on the session row.
	if _, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "escape-task", Directory: "../outside"}); err == nil {
		t.Fatal("expected escape to be rejected")
	}
	failed, err := store.LoadSession("escape-task")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != storage.StatusError || failed.Error == "" {
		t.Fatalf("spawn failure not recorded on session: %#v", failed)
	}
}

func TestSpawnWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "ink-backend")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@jaz"},
		{"config", "user.name", "jaz"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: workspace,
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
			},
		},
	}, log.New(io.Discard))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "wt-task", Directory: "ink-backend", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	want := filepath.Join(workspace, ".worktrees", spawned.Slug)
	if spawned.Cwd != want {
		t.Fatalf("cwd = %q, want %q", spawned.Cwd, want)
	}
	branch, err := exec.Command("git", "-C", want, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "jaz/"+spawned.Slug {
		t.Fatalf("worktree branch = %q", got)
	}

	// Worktree without a repository directory is an explicit error.
	if _, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "wt-bad", Directory: "not-a-repo", Worktree: true}); err == nil {
		t.Fatal("expected worktree on plain directory to fail")
	}
}
