package memorydream

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeManager struct {
	spawn     acp.SpawnRequest
	job       acp.Job
	send      acp.SendRequest
	finish    func()
	waitErr   error
	cancelled bool
}

func (f *fakeManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.spawn = req
	return acp.SpawnResult{SessionID: "dream-session"}, nil
}

func (f *fakeManager) Send(_ context.Context, req acp.SendRequest) (acp.Job, error) {
	f.send = req
	return acp.Job{State: acp.StateRunning}, nil
}

func (f *fakeManager) Wait(context.Context, acp.WaitRequest) (acp.Job, error) {
	if f.finish != nil {
		f.finish()
	}
	return f.job, f.waitErr
}

func (f *fakeManager) Cancel(context.Context, string) (acp.Job, error) {
	f.cancelled = true
	return acp.Job{State: acp.StateCancelled}, nil
}

func TestAgentPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt, err := agentPrompt(jazmem.DreamRequest{
		Root: "/tmp/memory",
		Date: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	}, "dreams/runs/test", "dreams/review/test", "/tmp/processed.json", "/tmp/sources.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LONG_TERM.md is profile memory",
		"routine coding style",
		"feature decisions",
		"weak one-off contacts",
		"SHORT_TERM.md is the active working set",
		"active working set at or below 5,000 characters",
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
		{name: "codex", agent: acp.AgentCodex, model: provider.DefaultOpenAIModel, effort: "low"},
		{name: "claude", agent: acp.AgentClaude, model: "default", effort: "low"},
		{name: "grok", agent: acp.AgentGrok},
		{name: "opencode-openrouter-style", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenRouter},
		}}, model: "z-ai/glm-5.2", effort: "low"},
		{name: "opencode-openai", agent: acp.AgentOpenCode, defaults: jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenAI},
		}}, model: provider.DefaultOpenAIModel, effort: "low"},
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
			root := t.TempDir()
			manager.finish = func() { finishDream(t, manager, "[]", true) }
			runner := New(store, manager, sourcequeue.New(root))

			_, err := runner.RunDream(context.Background(), jazmem.DreamRequest{
				Root: root,
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
	memory.SetDreamRunner(New(store, &fakeManager{}, sourcequeue.New(root)))

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

func finishDream(t *testing.T, manager *fakeManager, receipt string, report bool) {
	t.Helper()
	root, suffix := manager.spawn.Directory, manager.spawn.SourceID
	if report {
		file := filepath.Join(root, "dreams", "runs", suffix+".md")
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("# Consolidation\n\nChecked the supplied sources."), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if receipt != "" {
		file := filepath.Join(root, ".state", "consolidation", suffix+".json")
		if !strings.Contains(manager.send.Message, file) {
			t.Fatal("worker was not told where to acknowledge processed sources")
		}
		if err := os.WriteFile(file, []byte(receipt), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConsolidationPreservesUnfinishedAndConcurrentSourceUpdates(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	queue := sourcequeue.New(root)
	now := time.Now().UTC()
	for _, path := range []string{"sources/first.md", "sources/second.md"} {
		if err := queue.MarkPendingSource(t.Context(), sourcequeue.Source{Path: path, PendingAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeManager{job: acp.Job{State: acp.StateIdle}}
	manager.finish = func() {
		paths := workerSources(t, manager)
		for _, path := range []string{"sources/first.md", "sources/second.md"} {
			if !slices.Contains(paths, path) {
				t.Fatalf("worker missing queued source %s", path)
			}
		}
		if err := queue.MarkPendingSource(t.Context(), sourcequeue.Source{Path: "sources/first.md", PendingAt: now.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		finishDream(t, manager, `["sources/first.md"]`, true)
	}
	result, err := New(store, manager, queue).RunDream(t.Context(), jazmem.DreamRequest{Root: root})
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	stats, err := queue.Stats(t.Context())
	if err != nil || stats.Pending != 2 || stats.Processing != 0 {
		t.Fatalf("queue=%#v err=%v", stats, err)
	}
	reserved, err := queue.Reserve(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range reserved {
		if source.Path == "sources/first.md" && !source.PendingAt.Equal(now.Add(time.Minute)) {
			t.Fatal("newer source revision was lost")
		}
	}
}

func TestConsolidationLargeBacklogKeepsStartupPromptBounded(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	queue := sourcequeue.New(root)
	const count = 20000
	pending := make(map[string]map[string]time.Time, count)
	for i := range count {
		path := fmt.Sprintf("sources/email/gmail/personal/messages/2026-09-15-%032d.md", i)
		pending[path] = map[string]time.Time{"pending_at": time.Now().UTC()}
	}
	state, err := json.Marshal(map[string]any{"pending": pending})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".state", "pending-sources.json"), state, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{job: acp.Job{State: acp.StateIdle}}
	manager.finish = func() {
		if len(manager.send.Message) > 8192 {
			t.Fatalf("startup prompt grew to %d bytes for %d sources", len(manager.send.Message), count)
		}
		paths := workerSources(t, manager)
		if len(paths) != count {
			t.Fatalf("worker received %d sources, want %d", len(paths), count)
		}
		for _, path := range paths {
			if _, ok := pending[path]; !ok {
				t.Fatalf("unexpected or duplicate source %q", path)
			}
			delete(pending, path)
		}
		finishDream(t, manager, fmt.Sprintf("[%q]", paths[0]), true)
	}
	if _, err := New(store, manager, queue).RunDream(t.Context(), jazmem.DreamRequest{Root: root}); err != nil {
		t.Fatal(err)
	}
	stats, err := queue.Stats(t.Context())
	if err != nil || stats.Pending != count-1 || stats.Processing != 0 {
		t.Fatalf("queue=%#v err=%v", stats, err)
	}
	files, err := os.ReadDir(filepath.Join(root, ".state", "consolidation"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files remain: %v, err=%v", files, err)
	}
}

func workerSources(t *testing.T, manager *fakeManager) []string {
	t.Helper()
	file := filepath.Join(manager.spawn.Directory, ".state", "consolidation", manager.spawn.SourceID+".sources.json")
	if !strings.Contains(manager.send.Message, file) {
		t.Fatal("worker was not told where to read changed source paths")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil || paths == nil {
		t.Fatalf("invalid source list: %s, err=%v", data, err)
	}
	return paths
}

func TestConsolidationRequiresAcknowledgedWork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		receipt string
		report  bool
		state   string
	}{
		{"missing report", `["sources/a.md"]`, false, acp.StateIdle},
		{"missing receipt", "", true, acp.StateIdle},
		{"malformed receipt", "{", true, acp.StateIdle},
		{"null receipt", "null", true, acp.StateIdle},
		{"unreserved source", `["sources/other.md"]`, true, acp.StateIdle},
		{"duplicate source", `["sources/a.md","sources/a.md"]`, true, acp.StateIdle},
		{"failed worker", `["sources/a.md"]`, true, acp.StateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore(t)
			if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			queue := sourcequeue.New(root)
			if err := queue.MarkPendingSource(t.Context(), sourcequeue.Source{Path: "sources/a.md"}); err != nil {
				t.Fatal(err)
			}
			manager := &fakeManager{job: acp.Job{State: tc.state}}
			manager.finish = func() { finishDream(t, manager, tc.receipt, tc.report) }
			if _, err := New(store, manager, queue).RunDream(t.Context(), jazmem.DreamRequest{Root: root}); err == nil {
				t.Fatal("expected incomplete run to fail")
			}
			stats, err := queue.Stats(t.Context())
			if err != nil || stats.Pending != 1 || stats.Processing != 0 {
				t.Fatalf("queue=%#v err=%v", stats, err)
			}
		})
	}
}

func TestConsolidationSettlesOnlyAfterWorkerStops(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   string
		waitErr error
		pending int
	}{
		{"completed", acp.StateIdle, nil, 0},
		{"timed out", acp.StateRunning, nil, 1},
		{"cancelled wait", acp.StateRunning, context.Canceled, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore(t)
			if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			queue := sourcequeue.New(root)
			if err := queue.MarkPendingSource(t.Context(), sourcequeue.Source{Path: "sources/a.md"}); err != nil {
				t.Fatal(err)
			}
			manager := &fakeManager{job: acp.Job{State: tc.state}, waitErr: tc.waitErr}
			manager.finish = func() {
				finishDream(t, manager, `["sources/a.md"]`, true)
			}
			_, err := New(store, manager, queue).RunDream(t.Context(), jazmem.DreamRequest{Root: root})
			if (err != nil) != (tc.pending > 0) || manager.cancelled != (tc.pending > 0) {
				t.Fatalf("err=%v cancelled=%v", err, manager.cancelled)
			}
			stats, err := queue.Stats(t.Context())
			if err != nil || stats.Pending != tc.pending || stats.Processing != 0 {
				t.Fatalf("queue=%#v err=%v", stats, err)
			}
		})
	}
}

func TestConsolidationPreservesSourcesWhenWorkDirectoryCannotBeCreated(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	queue := sourcequeue.New(root)
	if err := queue.MarkPendingSource(t.Context(), sourcequeue.Source{Path: "sources/a.md"}); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, ".state", "consolidation")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{job: acp.Job{State: acp.StateIdle}}
	manager.finish = func() { finishDream(t, manager, `["sources/a.md"]`, true) }
	runner := New(store, manager, queue)
	if _, err := runner.RunDream(t.Context(), jazmem.DreamRequest{Root: root}); err == nil {
		t.Fatal("expected a work-directory error")
	}
	if manager.spawn.ACPAgent != "" {
		t.Fatal("worker started before its inputs could be written")
	}
	stats, err := queue.Stats(t.Context())
	if err != nil || stats.Pending != 1 || stats.Processing != 0 {
		t.Fatalf("queue=%#v err=%v", stats, err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.RunDream(t.Context(), jazmem.DreamRequest{Root: root}); err != nil {
		t.Fatal(err)
	}
	stats, err = queue.Stats(t.Context())
	if err != nil || stats.Pending != 0 || stats.Processing != 0 {
		t.Fatalf("queue=%#v err=%v", stats, err)
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
