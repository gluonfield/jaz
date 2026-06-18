package memorysearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memoryservice"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeManager struct {
	spawn   acp.SpawnRequest
	send    acp.SendRequest
	wait    acp.WaitRequest
	cancel  string
	job     acp.Job
	sendErr error
	waitErr error
}

func (f *fakeManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.spawn = req
	return acp.SpawnResult{SessionID: "search-session"}, nil
}

func (f *fakeManager) Send(_ context.Context, req acp.SendRequest) (acp.Job, error) {
	f.send = req
	if f.sendErr != nil {
		return acp.Job{}, f.sendErr
	}
	return acp.Job{State: acp.StateRunning}, nil
}

func (f *fakeManager) Wait(_ context.Context, req acp.WaitRequest) (acp.Job, error) {
	f.wait = req
	if f.waitErr != nil {
		return acp.Job{}, f.waitErr
	}
	return f.job, nil
}

func (f *fakeManager) Cancel(_ context.Context, session string) (acp.Job, error) {
	f.cancel = session
	return acp.Job{State: acp.StateCancelled}, nil
}

func TestSearchMemorySpawnsRestrictedSearchSession(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentCodex,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{
		job: acp.Job{State: acp.StateIdle, Assistant: `{"answer":"Jaz memory search works"}`},
	}
	now := time.Date(2026, 6, 17, 12, 30, 0, 0, time.UTC)
	runner := New(store, manager)
	runner.Now = func() time.Time { return now }

	answer, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{
		Query:    "  what should Jaz memory_search return?  ",
		Deep:     true,
		ParentID: "parent-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != manager.job.Assistant {
		t.Fatalf("answer = %q", answer)
	}

	stamp := fmt.Sprintf("%d", now.UnixNano())
	if manager.spawn.ParentID != "parent-session" {
		t.Fatalf("parent id = %q", manager.spawn.ParentID)
	}
	if manager.spawn.ACPAgent != acp.AgentCodex {
		t.Fatalf("agent = %q", manager.spawn.ACPAgent)
	}
	if manager.spawn.Model != "gpt-5.4-mini" || manager.spawn.ReasoningEffort != "minimal" {
		t.Fatalf("model/effort = %q/%q", manager.spawn.Model, manager.spawn.ReasoningEffort)
	}
	if manager.spawn.SourceType != storage.SourceMemorySearch || manager.spawn.SourceID != stamp {
		t.Fatalf("source = %q/%q", manager.spawn.SourceType, manager.spawn.SourceID)
	}
	if manager.spawn.MCPServerPolicy != acp.MCPServerPolicyMemorySearchWorker {
		t.Fatalf("mcp server policy = %q", manager.spawn.MCPServerPolicy)
	}
	if manager.spawn.Slug != "memory-search-codex-"+stamp {
		t.Fatalf("slug = %q", manager.spawn.Slug)
	}
	if manager.send.Session != "search-session" || manager.send.Completion != acp.CompletionInline {
		t.Fatalf("send = %#v", manager.send)
	}
	for _, want := range []string{
		"`jazmem_search_raw`",
		"`jazmem_get_page`",
		`"references"`,
		`"search_notes"`,
		"The caller requested a broader search.",
		"Question:\nwhat should Jaz memory_search return?",
	} {
		if !strings.Contains(manager.send.Message, want) {
			t.Fatalf("prompt missing %q:\n%s", want, manager.send.Message)
		}
	}
	if manager.wait.Session != "search-session" || manager.wait.Timeout != Timeout {
		t.Fatalf("wait = %#v", manager.wait)
	}
}

func TestSearchMemoryUsesUnifiedMemoryAgent(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentClaude,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{
		job: acp.Job{State: acp.StateIdle, Assistant: `{"answer":"from unified memory agent"}`},
	}
	runner := New(store, manager)

	if _, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "same agent"}); err != nil {
		t.Fatal(err)
	}
	if manager.spawn.ACPAgent != acp.AgentClaude {
		t.Fatalf("agent = %q", manager.spawn.ACPAgent)
	}
	if manager.spawn.Model != "sonnet" {
		t.Fatalf("model = %q", manager.spawn.Model)
	}
}

func TestSearchMemoryRejectsBuiltinJazAgent(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentJaz,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{job: acp.Job{State: acp.StateIdle, Assistant: "unused"}}
	runner := New(store, manager)

	_, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "anything"})
	if err == nil || !strings.Contains(err.Error(), "built-in Jaz") {
		t.Fatalf("err = %v", err)
	}
	if manager.spawn.ACPAgent != "" {
		t.Fatalf("spawned unexpectedly: %#v", manager.spawn)
	}
}

func TestSearchMemoryCancelsTimedOutWorker(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentGrok,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{
		job: acp.Job{State: acp.StateRunning},
	}
	runner := New(store, manager)

	_, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "slow search"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if manager.cancel != "search-session" {
		t.Fatalf("cancelled = %q", manager.cancel)
	}
}

func TestSearchMemoryCancelsWorkerWhenWaitFails(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentCodex,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{waitErr: context.Canceled}
	runner := New(store, manager)

	_, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "cancelled search"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if manager.cancel != "search-session" {
		t.Fatalf("cancelled = %q", manager.cancel)
	}
}

func TestSearchMemoryCancelsWorkerWhenSendFails(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentCodex,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{sendErr: errors.New("send failed")}
	runner := New(store, manager)

	_, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "send failure"})
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("err = %v", err)
	}
	if manager.cancel != "search-session" {
		t.Fatalf("cancelled = %q", manager.cancel)
	}
}

func TestSearchMemoryUsesOpenCodeProviderSpecificMiniModel(t *testing.T) {
	store := newStore(t)
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		acp.AgentOpenCode: {Enabled: true, ModelProvider: "openai"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{
		Enabled: true,
		Agent:   acp.AgentOpenCode,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{job: acp.Job{State: acp.StateIdle, Assistant: `{"answer":"ok"}`}}
	runner := New(store, manager)

	if _, err := runner.SearchMemory(context.Background(), memoryservice.AgenticSearchRequest{Query: "opencode"}); err != nil {
		t.Fatal(err)
	}
	if manager.spawn.ACPAgent != acp.AgentOpenCode {
		t.Fatalf("agent = %q", manager.spawn.ACPAgent)
	}
	if manager.spawn.Model != "gpt-5.4-mini" {
		t.Fatalf("model = %q", manager.spawn.Model)
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
