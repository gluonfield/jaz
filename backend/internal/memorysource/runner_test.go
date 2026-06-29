package memorysource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestRunOnceProcessesPendingSourcesInBatchAndClearsOnSuccess(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{
		ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentCodex: {Enabled: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	writeSource(t, root, "sources/telegram/personal/conversations/user-1/2026/06/27.md", "## 2026-06-27 UTC\n10:42:09 Alice: hello")
	writeSource(t, root, "sources/telegram/personal/contacts.md", "- Alice | @alice | telegram:user:1")
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &sourcequeue.Queue{Root: root, Now: func() time.Time { return now }}
	for _, path := range []string{
		"sources/telegram/personal/conversations/user-1/2026/06/27.md",
		"sources/telegram/personal/contacts.md",
	} {
		if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: path, PendingAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeSourceManager{job: acp.Job{State: acp.StateIdle}}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: manager, BatchFiles: 10, BatchChars: 100000}

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if manager.spawn.SourceType != storage.SourceMemorySource || manager.spawn.Directory != root {
		t.Fatalf("spawn = %#v", manager.spawn)
	}
	for _, want := range []string{
		"sources/telegram/personal/conversations/user-1/2026/06/27.md",
		"sources/telegram/personal/contacts.md",
		"10:42:09 Alice: hello",
	} {
		if !strings.Contains(manager.send.Message, want) {
			t.Fatalf("prompt missing %q:\n%s", want, manager.send.Message)
		}
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after success = %#v", pending)
	}
}

func TestRunUntilIdleProcessesMultipleAgentBatches(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{
		ACP: map[string]jazsettings.ACPAgentDefaults{
			acp.AgentCodex: {Enabled: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"sources/gmail/personal/messages/2026/06/27/a.md",
		"sources/gmail/personal/messages/2026/06/27/b.md",
		"sources/gmail/personal/messages/2026/06/27/c.md",
	}
	for _, path := range paths {
		writeSource(t, root, path, path)
	}
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &sourcequeue.Queue{Root: root, Now: func() time.Time { return now }}
	for _, path := range paths {
		if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: path, PendingAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeSourceManager{job: acp.Job{State: acp.StateIdle}}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: manager, BatchFiles: 2, BatchChars: 100000}

	processed, err := runner.RunUntilIdle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 3 {
		t.Fatalf("processed = %d, want 3", processed)
	}
	if len(manager.sendMessages) != 2 {
		t.Fatalf("agent batches = %d, want 2", len(manager.sendMessages))
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("memory source queue was not drained: %#v", pending)
	}
}

func TestRunOnceLeavesPendingSourcesWhenAgentFails(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{acp.AgentCodex: {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	path := "sources/gmail/personal/messages/2026/06/27/m1.md"
	writeSource(t, root, path, "mail")
	queue := sourcequeue.New(root)
	if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: path}); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: &fakeSourceManager{job: acp.Job{State: acp.StateFailed, Error: "boom"}}}

	if _, err := runner.RunOnce(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Path != path {
		t.Fatalf("pending after failure = %#v", pending)
	}
}

func TestRunOnceCompletesTruncatedSourcesAfterSuccessfulAgentRun(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{acp.AgentCodex: {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	path := "sources/telegram/personal/conversations/user-1/2026/06/27.md"
	writeSource(t, root, path, strings.Repeat("x", 20))
	queue := sourcequeue.New(root)
	if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: path}); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: &fakeSourceManager{job: acp.Job{State: acp.StateIdle}}, BatchChars: 4}

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("truncated source was retried: %#v", pending)
	}
}

func TestRunOnceReleasesReservedSourcesOutsideCharBudget(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{acp.AgentCodex: {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"sources/gmail/personal/messages/2026/06/27/a.md",
		"sources/gmail/personal/messages/2026/06/27/b.md",
		"sources/gmail/personal/messages/2026/06/27/c.md",
	}
	for _, path := range paths {
		writeSource(t, root, path, strings.Repeat("x", 8))
	}
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &sourcequeue.Queue{Root: root, Now: func() time.Time { return now }}
	for _, path := range paths {
		if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: path, PendingAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: &fakeSourceManager{job: acp.Job{State: acp.StateIdle}}, BatchFiles: 3, BatchChars: 12}

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].Path != paths[1] || pending[1].Path != paths[2] {
		t.Fatalf("deferred pending sources = %#v", pending)
	}
}

func TestRunOnceDropsMissingSourcesWithoutSpawning(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := jazsettings.SaveMemorySettings(store, jazsettings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{acp.AgentCodex: {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	queue := sourcequeue.New(root)
	if err := queue.MarkPendingSource(context.Background(), sourcequeue.Source{Path: "sources/gmail/personal/messages/2026/06/27/gone.md"}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeSourceManager{job: acp.Job{State: acp.StateIdle}}
	runner := &Runner{Root: root, Store: store, Queue: queue, Manager: manager}

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1 (dropped ghost)", processed)
	}
	if manager.spawn.SourceType != "" {
		t.Fatalf("agent was spawned for a missing file: %#v", manager.spawn)
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("missing source was not dropped: %#v", pending)
	}
}

func writeSource(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fakeSourceManager struct {
	spawn        acp.SpawnRequest
	send         acp.SendRequest
	sendMessages []string
	job          acp.Job
}

func (f *fakeSourceManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.spawn = req
	return acp.SpawnResult{SessionID: "source-session"}, nil
}

func (f *fakeSourceManager) Send(_ context.Context, req acp.SendRequest) (acp.Job, error) {
	f.send = req
	f.sendMessages = append(f.sendMessages, req.Message)
	return acp.Job{State: acp.StateRunning}, nil
}

func (f *fakeSourceManager) Wait(context.Context, acp.WaitRequest) (acp.Job, error) {
	return f.job, nil
}

func (f *fakeSourceManager) Cancel(context.Context, string) (acp.Job, error) {
	return acp.Job{State: acp.StateCancelled}, nil
}
