package acp_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type utilityLocalRunner struct {
	seen    chan acp.LocalUtilityRequest
	text    string
	errText string
	drained chan struct{}
	stall   bool
}

func (r utilityLocalRunner) Run(context.Context, acp.LocalAgentRequest) <-chan agent.StreamEvent {
	out := make(chan agent.StreamEvent)
	close(out)
	return out
}

func (r utilityLocalRunner) RunUtility(_ context.Context, req acp.LocalUtilityRequest) <-chan agent.StreamEvent {
	out := make(chan agent.StreamEvent)
	if r.stall {
		return out
	}
	go func() {
		defer close(out)
		if r.seen != nil {
			r.seen <- req
		}
		if r.errText != "" {
			out <- agent.StreamEvent{Type: agent.StreamError, Error: r.errText}
			out <- agent.StreamEvent{Type: agent.StreamDone}
			if r.drained != nil {
				close(r.drained)
			}
			return
		}
		text := r.text
		if text == "" {
			text = "local utility reply"
		}
		out <- agent.StreamEvent{Type: agent.StreamDelta, Delta: text}
		out <- agent.StreamEvent{Type: agent.StreamDone}
	}()
	return out
}

func newUtilityLocalManager(t *testing.T, runner acp.LocalAgentRunner, cfg acp.AgentConfig) (*jsonstore.Store, *acp.Manager) {
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents: map[string]acp.AgentConfig{
			acp.AgentJaz: cfg,
		},
	}, log.New(io.Discard))
	manager.RegisterLocalAgent(acp.AgentJaz, runner)
	return store, manager
}

func TestManagerRunUtilityPromptUsesACPWithoutPersistingSession(t *testing.T) {
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
				Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1"},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	text, err := manager.RunUtilityPrompt(ctx, acp.UtilityPromptRequest{
		ACPAgent:  "fake",
		Directory: ".",
		Message:   "return a short title",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello from fake agent" {
		t.Fatalf("text = %q, want fake agent output", text)
	}
	if jobs := manager.List(); len(jobs) != 0 {
		t.Fatalf("jobs = %#v, want no registered job", jobs)
	}
	sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want no stored thread", sessions)
	}
}

func TestManagerRunUtilityPromptUsesLocalRunnerWithoutPersistingSession(t *testing.T) {
	runner := utilityLocalRunner{
		seen: make(chan acp.LocalUtilityRequest, 1),
		text: `{"title":"Local Utility Title"}`,
	}
	store, manager := newUtilityLocalManager(t, runner, acp.AgentConfig{
		Local:           true,
		ModelProvider:   "openrouter",
		Model:           "openai/gpt-test",
		ReasoningEffort: "medium",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	text, err := manager.RunUtilityPrompt(ctx, acp.UtilityPromptRequest{
		ACPAgent:  acp.AgentJaz,
		Directory: ".",
		Message:   "return a short title",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != `{"title":"Local Utility Title"}` {
		t.Fatalf("text = %q, want local utility output", text)
	}
	select {
	case req := <-runner.seen:
		if req.Message != "return a short title" ||
			req.Session.ModelProvider != "openrouter" ||
			req.Session.Model != "openai/gpt-test" ||
			req.Session.ReasoningEffort != "medium" ||
			req.Session.RuntimeRef == nil ||
			req.Session.RuntimeRef.Agent != acp.AgentJaz ||
			req.Session.RuntimeRef.Cwd == "" {
			t.Fatalf("local utility request = %#v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("local utility runner was not called")
	}
	if jobs := manager.List(); len(jobs) != 0 {
		t.Fatalf("jobs = %#v, want no registered job", jobs)
	}
	sessions, err := store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want no stored thread", sessions)
	}
}

func TestManagerRunUtilityPromptDrainsLocalErrorStream(t *testing.T) {
	drained := make(chan struct{})
	runner := utilityLocalRunner{errText: "boom", drained: drained}
	_, manager := newUtilityLocalManager(t, runner, acp.AgentConfig{Local: true})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := manager.RunUtilityPrompt(ctx, acp.UtilityPromptRequest{
		ACPAgent:  acp.AgentJaz,
		Directory: ".",
		Message:   "return a short title",
	})
	if err == nil || err.Error() != "local utility prompt failed: boom" {
		t.Fatalf("err = %v, want local utility error", err)
	}
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("local utility stream was not drained after error")
	}
}

func TestManagerRunUtilityPromptTimesOutStalledLocalStream(t *testing.T) {
	_, manager := newUtilityLocalManager(t, utilityLocalRunner{stall: true}, acp.AgentConfig{Local: true})

	_, err := manager.RunUtilityPrompt(context.Background(), acp.UtilityPromptRequest{
		ACPAgent:  acp.AgentJaz,
		Directory: ".",
		Message:   "return a short title",
		Timeout:   20 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}
