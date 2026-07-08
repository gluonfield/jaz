package memorydream

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeManager struct {
	spawn acp.SpawnRequest
	job   acp.Job
}

func (f *fakeManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.spawn = req
	return acp.SpawnResult{SessionID: "dream-session"}, nil
}

func (f *fakeManager) Send(context.Context, acp.SendRequest) (acp.Job, error) {
	return acp.Job{State: acp.StateRunning}, nil
}

func (f *fakeManager) Wait(context.Context, acp.WaitRequest) (acp.Job, error) {
	return f.job, nil
}

func (f *fakeManager) Cancel(context.Context, string) (acp.Job, error) {
	return acp.Job{State: acp.StateCancelled}, nil
}

func TestAgentPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt, err := agentPrompt(jazmem.DreamRequest{
		Root: "/tmp/memory",
		Date: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	}, "dreams/runs/test", "dreams/review/test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LONG_TERM.md is profile memory",
		"routine coding style",
		"feature decisions",
		"weak one-off contacts",
		"SHORT_TERM.md is the active working set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("agent prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRunDreamSpawnsCompatibleWorkerModelAndEffort(t *testing.T) {
	cases := []struct {
		name     string
		agent    string
		defaults jazsettings.AgentDefaults
		model    string
		effort   string
	}{
		{name: "codex", agent: acp.AgentCodex, model: "gpt-5.4-mini", effort: "xhigh"},
		{name: "claude", agent: acp.AgentClaude, model: "default", effort: "xhigh"},
		{name: "grok", agent: acp.AgentGrok, model: "grok-composer-2.5-fast", effort: "xhigh"},
		{name: "opencode-openrouter-style", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenRouter},
		}}, model: "z-ai/glm-5.2", effort: "xhigh"},
		{name: "opencode-openai", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenAI},
		}}, model: "gpt-5.4-mini", effort: "xhigh"},
		{name: "opencode-ollama", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: provider.ProviderOllama},
		}}},
		{name: "opencode-custom-provider", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: "internal"},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore(t)
			if tc.defaults.ACP != nil {
				if _, err := jazsettings.SaveAgentDefaults(store, tc.defaults); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
				Enabled: true,
				Agent:   tc.agent,
			}); err != nil {
				t.Fatal(err)
			}
			manager := &fakeManager{job: acp.Job{State: acp.StateIdle, Assistant: "done"}}
			runner := New(store, manager, nil)

			_, err := runner.RunDream(context.Background(), jazmem.DreamRequest{
				Root: t.TempDir(),
				Date: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			if manager.spawn.ACPAgent != tc.agent || manager.spawn.Model != tc.model || manager.spawn.ReasoningEffort != tc.effort {
				t.Fatalf("spawn = agent %q model %q effort %q, want %q/%q/%q",
					manager.spawn.ACPAgent,
					manager.spawn.Model,
					manager.spawn.ReasoningEffort,
					tc.agent,
					tc.model,
					tc.effort,
				)
			}
		})
	}
}

func TestRunDreamTaskWithoutMemoryAgentDoesNotFallBackToOpenRouter(t *testing.T) {
	store := newStore(t)
	root := t.TempDir()
	memory, err := jazmem.Open(jazmem.Config{
		Root:   root,
		DBPath: filepath.Join(t.TempDir(), "index.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	memory.SetDreamRunner(New(store, &fakeManager{}, nil))

	_, err = memory.RunDreamTask(context.Background(), jazmem.DreamOptions{})
	if err == nil {
		t.Fatal("expected missing memory agent error")
	}
	if !strings.Contains(err.Error(), "memory agent is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "OPENROUTER") {
		t.Fatalf("dream fell through to provider-backed fallback: %v", err)
	}
}

func newStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
