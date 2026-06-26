package acp_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// staticPrompt is a fixed acp.SystemPromptSource for tests.
type staticPrompt string

func (s staticPrompt) ACPPromptForContext(_ context.Context, _, _ string) (string, error) {
	return string(s), nil
}

type cwdPrompt struct{}

func (cwdPrompt) ACPPromptForContext(_ context.Context, cwd, _ string) (string, error) {
	return "cwd: " + cwd, nil
}

type localRunner struct {
	seen     chan acp.LocalAgentRequest
	platform chan string
}

func (r localRunner) Run(ctx context.Context, req acp.LocalAgentRequest) <-chan agent.StreamEvent {
	out := make(chan agent.StreamEvent, 4)
	go func() {
		defer close(out)
		if r.platform != nil {
			r.platform <- sessioncontext.ClientPlatform(ctx)
		}
		r.seen <- req
		select {
		case <-ctx.Done():
			out <- agent.StreamEvent{Type: agent.StreamError, Error: ctx.Err().Error()}
			return
		default:
		}
		out <- agent.StreamEvent{Type: agent.StreamDelta, Delta: "local reply"}
		call := provider.FunctionToolCall("tool-1", "inspect", "{}")
		out <- agent.StreamEvent{Type: agent.StreamToolCall, ToolCall: &call}
		out <- agent.StreamEvent{Type: agent.StreamToolResult, ToolName: "inspect"}
		out <- agent.StreamEvent{Type: agent.StreamDone, Usage: &provider.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}}
	}()
	return out
}

func TestManagerSpawnsFakeACPAgentAndStoresSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "main", Runtime: storage.RuntimeACP})
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
	if status.Modes.PlanModeID != "plan" || status.Modes.CurrentModeID != "full-access" {
		t.Fatalf("unexpected modes %#v", status.Modes)
	}
	if status.ModelProvider != "fake" || status.Model != "fake-large" || status.ReasoningEffort != "high" {
		t.Fatalf("unexpected status model metadata %#v", status)
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
	state, err := store.LoadACPState(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ModelProvider != "fake" || state.Model != "fake-large" || state.ReasoningEffort != "high" {
		t.Fatalf("unexpected acp state model metadata %#v", state)
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

	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:       session.RuntimeRef.SessionID,
		Message:       "again",
		Completion:    acp.CompletionAsync,
		ParentVisible: true,
	}); err != nil {
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

func TestManagerCompactUsesHiddenCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeCodexManager(t, store, t.TempDir(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "codex-compact"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	job, err := manager.Compact(ctx, acp.CompactRequest{Session: spawned.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if job.ActiveOperation != acp.ActiveOperationCompact {
		t.Fatalf("active operation = %q, want compact", job.ActiveOperation)
	}
	if _, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("stored messages = %#v, want none", messages)
	}
}

func TestManagerCompactRejectsUnsupportedAgent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-compact"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Compact(ctx, acp.CompactRequest{Session: spawned.SessionID}); err == nil || !strings.Contains(err.Error(), "compact is not available") {
		t.Fatalf("compact error = %v, want unsupported compact", err)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("stored messages = %#v, want none", messages)
	}
}

type fakeAdapterResolver struct {
	launch acp.AdapterLaunch
}

func (r fakeAdapterResolver) ResolveAdapter(context.Context, string) (acp.AdapterLaunch, error) {
	return r.launch, nil
}

func TestManagerSpawnsManagedAdapterAgent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Adapters: fakeAdapterResolver{launch: acp.AdapterLaunch{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeACPAgentProcess"},
		}},
		Agents: map[string]acp.AgentConfig{
			"fake": {
				ManagedAdapter: "fake",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT": "1",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "managed-fake"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	if spawned.State != acp.StateIdle {
		t.Fatalf("spawn state = %s, want %s", spawned.State, acp.StateIdle)
	}
}

func TestManagerRunsLocalJazAgentThroughACPJob(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := localRunner{
		seen:     make(chan acp.LocalAgentRequest, 1),
		platform: make(chan string, 1),
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			acp.AgentJaz: {
				Local:           true,
				ModelProvider:   "openrouter",
				Model:           "openai/gpt-test",
				ReasoningEffort: "medium",
			},
		},
	}, log.New(io.Discard))
	manager.RegisterLocalAgent(acp.AgentJaz, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{Slug: "local-jaz"})
	if err != nil {
		t.Fatal(err)
	}
	if spawned.ACPAgent != acp.AgentJaz || spawned.State != acp.StateIdle {
		t.Fatalf("spawned = %#v", spawned)
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Runtime != storage.RuntimeACP ||
		session.RuntimeRef.Agent != acp.AgentJaz ||
		session.RuntimeRef.SessionID != session.ID ||
		session.ModelProvider != "openrouter" ||
		session.Model != "openai/gpt-test" {
		t.Fatalf("local session metadata = %#v", session)
	}

	mobileCtx := sessioncontext.WithClientPlatform(ctx, "mobile")
	if _, err := manager.Send(mobileCtx, acp.SendRequest{
		Session:       spawned.SessionID,
		Message:       "make a plan",
		PlanRequested: true,
		Completion:    acp.CompletionInline,
	}); err != nil {
		t.Fatal(err)
	}
	if platform := <-runner.platform; platform != "mobile" {
		t.Fatalf("local runner platform = %q, want mobile", platform)
	}
	req := <-runner.seen
	if req.Session.ID != spawned.SessionID || req.Message != "make a plan" || !req.PlanRequested {
		t.Fatalf("local runner request = %#v", req)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Assistant != "local reply" {
		t.Fatalf("job = %#v", job)
	}
	if len(job.ToolCalls) != 1 || job.ToolCalls[0].ID != "tool-1" || job.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool calls = %#v", job.ToolCalls)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || provider.MessageContent(messages[0]) != "make a plan" {
		t.Fatalf("stored messages = %#v", messages)
	}
	session, err = store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Usage.InputTokens != 3 || session.Usage.OutputTokens != 5 || session.Usage.TotalTokens != 8 {
		t.Fatalf("usage = %#v", session.Usage)
	}
}

func TestManagerSideChatDoesNotTouchRunningTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeCodexManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_EXPECT_PROMPT_TRIGGER":  "quick check",
		"JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": "selected text",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "codex-side"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	manager.Events = sessionevents.New()
	live := manager.Events.Subscribe(ctx, spawned.SessionID)
	if err := manager.SendSideChat(ctx, acp.SideChatRequest{
		Session:  spawned.SessionID,
		ID:       "side-1",
		Message:  "quick check",
		Contexts: storage.SelectionContexts([]string{"selected text"}),
	}); err != nil {
		t.Fatal(err)
	}
	sideEvents := collectSideChatEvents(t, live, 2)

	job, err := manager.Status(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateRunning || job.Assistant != "" {
		t.Fatalf("main turn changed: state=%s assistant=%q", job.State, job.Assistant)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || provider.MessageContent(messages[0]) != "block until cancelled" {
		t.Fatalf("side chat leaked into messages %#v", messages)
	}
	if !hasSideChatEvent(sideEvents, "side-1", "user", "quick check") ||
		!hasSideChatEvent(sideEvents, "side-1", "assistant", "hello from side chat") {
		t.Fatalf("missing side chat events %#v", sideEvents)
	}
	userEvent := sideChatEvent(sideEvents, "side-1", "user", "quick check")
	if userEvent == nil || len(userEvent.Contexts) != 1 || userEvent.Contexts[0].Text != "selected text" {
		t.Fatalf("side chat user event contexts = %#v", userEvent)
	}
	storedEvents, err := store.LoadSessionEvents(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSideChatEvent(storedEvents, "side-1", "user", "quick check") ||
		!hasSideChatEvent(storedEvents, "side-1", "assistant", "hello from side chat") {
		t.Fatalf("side chat did not persist in session events %#v", storedEvents)
	}
	if hasACPMessage(storedEvents, "hello from side chat") {
		t.Fatalf("side chat leaked into main acp transcript %#v", storedEvents)
	}
}

func TestManagerSteerUsesPromptQueueingWithoutCancel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_PROMPT_QUEUEING": "1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-follow"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "say hello"}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.StopReason == "cancelled" || job.Assistant != "hello from fake agent" {
		t.Fatalf("steered job state=%s stop=%q assistant=%q error=%q", job.State, job.StopReason, job.Assistant, job.Error)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 ||
		provider.MessageContent(messages[0]) != "block until cancelled" ||
		provider.MessageContent(messages[1]) != "say hello" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestManagerSteerIncludesMessageContext(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_PROMPT_QUEUEING":        "1",
		"JAZ_FAKE_ACP_EXPECT_PROMPT_TRIGGER":  "apply context",
		"JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": "selected context",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-follow-context"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "block until cancelled", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Steer(ctx, acp.SteerRequest{
		Session:  spawned.SessionID,
		Message:  "apply context",
		Contexts: storage.SelectionContexts([]string{"selected context"}),
	}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle {
		t.Fatalf("state=%s error=%q", job.State, job.Error)
	}
}

func TestManagerPassesResolvedCwdToACPPrompt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	want := filepath.Join(workspace, "project")
	manager := acp.NewManager(store, acp.Config{
		Root:         t.TempDir(),
		Workspace:    workspace,
		SystemPrompt: cwdPrompt{},
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":         "1",
					"JAZ_FAKE_ACP_SYSTEM_PROMPT": "cwd: " + want,
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "cwd-prompt", Directory: "project"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	if spawned.Cwd != want {
		t.Fatalf("cwd = %q, want %q", spawned.Cwd, want)
	}
}

func TestManagerSendStartsStoredACPSession(t *testing.T) {
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

	session, err := manager.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:  "fake",
		Slug:      "fake-stored",
		Directory: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.SessionID != "" || session.RuntimeRef.Cwd != workspace {
		t.Fatalf("stored runtime ref = %#v", session.RuntimeRef)
	}
	status, err := manager.Status(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "not_running" {
		t.Fatalf("stored status = %s, want not_running", status.State)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: session.ID, Message: "say hello", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), session.ID) }()
	if job.ACPSession == "" || job.State != acp.StateIdle {
		t.Fatalf("job = %#v", job)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef == nil || loaded.RuntimeRef.SessionID == "" {
		t.Fatalf("loaded runtime ref = %#v", loaded.RuntimeRef)
	}
}

func TestManagerIncludesAgentStderrWhenInitializeConnectionCloses(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":            "1",
					"JAZ_FAKE_ACP_EXIT_BEFORE_INIT": "claude: command not found",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-crash"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), "initialize acp agent") || !strings.Contains(err.Error(), "claude: command not found") {
		t.Fatalf("error = %q", err)
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

func TestManagerUsesClaudeMaxEffortConfigOption(t *testing.T) {
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
				ReasoningEffort: "max",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "default,sonnet",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "default",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":    "effort",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "max",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "claude", Slug: "claude-max-effort"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ReasoningEffort != "max" {
		t.Fatalf("claude effort = %q, want max", session.ReasoningEffort)
	}
}

func TestManagerUsesClaudeUltracodeSessionMeta(t *testing.T) {
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
				ReasoningEffort: "ultracode",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "default,sonnet",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "default",
					"JAZ_FAKE_ACP_EXPECT_ULTRACODE":    "1",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":    "effort",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "xhigh",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "claude", Slug: "claude-ultracode"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ReasoningEffort != "ultracode" {
		t.Fatalf("claude effort = %q, want ultracode", session.ReasoningEffort)
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

func TestManagerUsesCodexModelAndEffortConfigOptions(t *testing.T) {
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
				ProviderMode:    acp.AgentProviderModeAgentDefaults,
				ModelProvider:   provider.ProviderOpenAI,
				Model:           "fake-large",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "fake-large",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "xhigh",
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

func TestManagerUsesCodexProviderNativeModel(t *testing.T) {
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
				Args:            []string{"-test.run=TestFakeACPAgentProcess", "--"},
				ProviderMode:    acp.AgentProviderModeAgentDefaults,
				ModelProvider:   provider.ProviderOpenRouter,
				Model:           "openai/gpt-5.5",
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "openai/gpt-5.5",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "xhigh",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "codex-openrouter-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ModelProvider != provider.ProviderOpenRouter || session.Model != "openai/gpt-5.5" || session.ReasoningEffort != "xhigh" {
		t.Fatalf("unexpected stored model metadata %#v", session)
	}
}

func TestManagerUsesAdvertisedThoughtLevelConfigOption(t *testing.T) {
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
				ReasoningEffort: "high",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":                       "1",
					"JAZ_FAKE_ACP_MODELS":                      "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG":         "fake-large",
					"JAZ_FAKE_ACP_SET_CONFIG":                  "1",
					"JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_ID":      "thinking_budget",
					"JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_OPTIONS": "high",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":            "thinking_budget",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":               "high",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "codex-thought-level"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
}

func TestManagerSpawnModelOverrideWinsOverConfiguredModel(t *testing.T) {
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
				ReasoningEffort: "medium",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "fake-large,fake-mini",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "fake-mini",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "medium",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{
		ACPAgent: "codex",
		Slug:     "codex-model-override",
		Model:    "fake-mini",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Model != "fake-mini" || session.ReasoningEffort != "medium" {
		t.Fatalf("unexpected stored model metadata %#v", session)
	}
}

func TestManagerAllowsUnadvertisedCodexModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
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
				Model:   "missing-model",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":                     "1",
					"JAZ_FAKE_ACP_MODELS":                    "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG":       "missing-model",
					"JAZ_FAKE_ACP_SET_CONFIG":                "1",
					"JAZ_FAKE_ACP_MODEL_CONFIG_OMITS_EFFORT": "1",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "missing-codex-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Model != "missing-model" || session.ReasoningEffort != "" {
		t.Fatalf("unexpected stored model metadata %#v", session)
	}
}

func TestManagerSkipsConfiguredCodexEffortWhenModelOmitsEffortConfig(t *testing.T) {
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
				ReasoningEffort: "xhigh",
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":                     "1",
					"JAZ_FAKE_ACP_MODELS":                    "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG":       "missing-model",
					"JAZ_FAKE_ACP_SET_CONFIG":                "1",
					"JAZ_FAKE_ACP_MODEL_CONFIG_OMITS_EFFORT": "1",
					"JAZ_FAKE_ACP_EXPECT_CONFIG_ID":          "unexpected_effort_config",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "missing-codex-effort-config"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Status == storage.StatusError || session.Error != "" {
		t.Fatalf("missing effort config should not fail session: %#v", session)
	}
}

func TestManagerRejectsUnadvertisedReasoningEffort(t *testing.T) {
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
					"JAZ_FAKE_ACP_AGENT":                       "1",
					"JAZ_FAKE_ACP_MODELS":                      "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG":         "fake-large",
					"JAZ_FAKE_ACP_SET_CONFIG":                  "1",
					"JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_ID":      "thinking_budget",
					"JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_OPTIONS": "high",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "codex", Slug: "codex-unadvertised-effort"})
	if err == nil {
		t.Fatal("expected spawn to fail")
	}
	if !strings.Contains(err.Error(), `reasoning effort "xhigh"`) || !strings.Contains(err.Error(), "did not advertise that reasoning effort") {
		t.Fatalf("unexpected error: %v", err)
	}
	session, loadErr := store.LoadSession("codex-unadvertised-effort")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if session.Status != storage.StatusError || !strings.Contains(session.Error, "did not advertise that reasoning effort") {
		t.Fatalf("unadvertised effort failure was not stored: %#v", session)
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
					"JAZ_FAKE_ACP_AGENT":               "1",
					"JAZ_FAKE_ACP_MODELS":              "fake-large",
					"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "fake-large",
					"JAZ_FAKE_ACP_SET_CONFIG":          "1",
					"JAZ_FAKE_ACP_EXPECT_EFFORT":       "medium",
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
	if !strings.Contains(err.Error(), `reasoning effort "xhigh"`) || !strings.Contains(err.Error(), "expected configured reasoning effort") {
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

func collectSideChatEvents(t *testing.T, ch <-chan sessionevents.Event, want int) []sessionevents.Event {
	t.Helper()
	deadline := time.After(time.Second)
	var events []sessionevents.Event
	for len(events) < want {
		select {
		case event := <-ch:
			if event.Type == sessionevents.TypeSideChatMessage {
				events = append(events, event)
			}
		case <-deadline:
			t.Fatalf("side chat events = %#v, want %d", events, want)
		}
	}
	return events
}

func hasSideChatEvent(events []sessionevents.Event, id, role, content string) bool {
	return sideChatEvent(events, id, role, content) != nil
}

func sideChatEvent(events []sessionevents.Event, id, role, content string) *sessionevents.SideChatEvent {
	for _, event := range events {
		if event.Type == sessionevents.TypeSideChatMessage &&
			event.SideChat != nil &&
			event.SideChat.ID == id &&
			event.SideChat.Role == role &&
			event.SideChat.Content == content {
			return event.SideChat
		}
	}
	return nil
}

func newFakeAgentManager(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string) *acp.Manager {
	return newFakeAgentManagerWithModel(t, store, root, extraEnv, "")
}

func newFakeAgentManagerWithModel(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string, model string) *acp.Manager {
	return newFakeAgentManagerWithOptions(t, store, root, extraEnv, model, "")
}

func newFakeAgentManagerWithOptions(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string, model, effort string) *acp.Manager {
	return newFakeNamedAgentManagerWithOptions(t, store, root, "fake", extraEnv, model, effort)
}

func newFakeCodexManager(t *testing.T, store *jsonstore.Store, root string, extraEnv map[string]string) *acp.Manager {
	return newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, extraEnv, "", "")
}

func newFakeNamedAgentManagerWithOptions(t *testing.T, store *jsonstore.Store, root, agent string, extraEnv map[string]string, model, effort string) *acp.Manager {
	env := map[string]string{"JAZ_FAKE_ACP_AGENT": "1"}
	for key, value := range extraEnv {
		env[key] = value
	}
	return acp.NewManager(store, acp.Config{
		Root:      root,
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			agent: {
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

func TestManagerResumesStoredSessionAfterServeError(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manager := newFakeAgentManager(t, store, root, map[string]string{
		"JAZ_FAKE_ACP_LOAD": "1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-serve-error"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "break transport", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	failed, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != acp.StateFailed || !strings.Contains(failed.Error, "invalid character") {
		t.Fatalf("failed turn state=%s error=%q", failed.State, failed.Error)
	}

	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "after transport failure", Completion: acp.CompletionInline}); err != nil {
		t.Fatalf("send after serve error: %v", err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
		t.Fatalf("resumed turn state=%s assistant=%q error=%q", job.State, job.Assistant, job.Error)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || provider.MessageContent(messages[1]) != "after transport failure" {
		t.Fatalf("unexpected messages %#v", messages)
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

func TestCancelTreatsNormalStopAfterCancelAsCancelled(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_CANCEL_END_TURN": "1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-cancel-end-turn"})
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
	if job.State != acp.StateCancelled || job.StopReason != "cancelled" {
		t.Fatalf("state/stop_reason = %s/%q, want cancelled/cancelled", job.State, job.StopReason)
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
	var blockCancelFinished sync.Once
	manager.TurnFinished = func(_ context.Context, job acp.Job) {
		if job.ID == spawned.SessionID && job.State == acp.StateCancelled {
			blockCancelFinished.Do(func() {
				close(cancelFinishedEntered)
				<-releaseCancelFinished
			})
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
	if session.RuntimeRef == nil || session.RuntimeRef.Cwd != want || session.RuntimeRef.ProjectPath != want {
		t.Fatalf("cwd not persisted: %#v", session.RuntimeRef)
	}

	absolute := filepath.Join(t.TempDir(), "outside-project")
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		t.Fatal(err)
	}
	spawned, err = manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "absolute-dir-task", Directory: absolute})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
	if spawned.Cwd != absolute {
		t.Fatalf("absolute cwd = %q, want %q", spawned.Cwd, absolute)
	}
	session, err = store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.ProjectPath != absolute {
		t.Fatalf("absolute project path not persisted: %#v", session.RuntimeRef)
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

	if _, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "branch-without-worktree", Directory: "ink-backend", Branch: "main"}); err == nil {
		t.Fatal("expected branch without worktree to fail")
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
		{"init", "-b", "main"},
		{"config", "user.email", "test@jaz"},
		{"config", "user.name", "jaz"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	for _, args := range [][]string{
		{"switch", "-q", "-c", "feature"},
		{"commit", "--allow-empty", "-m", "feature"},
		{"switch", "-q", "main"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	manager := acp.NewManager(store, acp.Config{
		Root:         t.TempDir(),
		Workspace:    workspace,
		SystemPrompt: cwdPrompt{},
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":                "1",
					"JAZ_FAKE_ACP_EXPECT_CWD_IN_PROMPT": "1",
				},
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
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	wantRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Cwd != want || session.RuntimeRef.ProjectPath != wantRepo {
		t.Fatalf("worktree runtime ref = %#v, want cwd %q project %q", session.RuntimeRef, want, wantRepo)
	}

	branchSpawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "wt-feature", Directory: "ink-backend", Worktree: true, Branch: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), branchSpawned.SessionID) }()
	branchBase, err := exec.Command("git", "-C", branchSpawned.Cwd, "merge-base", "HEAD", "feature").Output()
	if err != nil {
		t.Fatal(err)
	}
	featureHead, err := exec.Command("git", "-C", branchSpawned.Cwd, "rev-parse", "feature").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branchBase)) != strings.TrimSpace(string(featureHead)) {
		t.Fatalf("branch worktree did not start from feature")
	}

	// Worktree without a repository directory is an explicit error.
	if _, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "wt-bad", Directory: "not-a-repo", Worktree: true}); err == nil {
		t.Fatal("expected worktree on plain directory to fail")
	}

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@jaz"},
		{"config", "user.name", "jaz"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git workspace %v: %v: %s", args, err, out)
		}
	}
	rootSpawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "wt-root", Directory: ".", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), rootSpawned.SessionID) }()
	rootWant := filepath.Join(workspace, ".worktrees", rootSpawned.Slug)
	session, err = store.LoadSession(rootSpawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	wantWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef == nil || session.RuntimeRef.Cwd != rootWant || session.RuntimeRef.ProjectPath != wantWorkspace {
		t.Fatalf("root worktree runtime ref = %#v, want cwd %q project %q", session.RuntimeRef, rootWant, wantWorkspace)
	}
}
