package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestLoopAdvanceMissedSkipsCatchUp(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := base
	service := newLoopServiceForTest(store, nil)
	service.Now = func() time.Time { return now }
	loop, err := service.Create(loops.CreateLoop{
		Name:     "Every minute",
		Prompt:   "check the thing",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loop.NextRunAt, base.Add(time.Minute); !got.Equal(want) {
		t.Fatalf("next_run_at = %s, want %s", got, want)
	}

	now = base.Add(5 * time.Minute)
	if err := service.AdvanceMissed(); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadLoop(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loaded.NextRunAt, base.Add(6*time.Minute); !got.Equal(want) {
		t.Fatalf("next_run_at after missed = %s, want %s", got, want)
	}
	runs, err := store.ListRuns(loop.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("missed schedules should not create runs: %#v", runs)
	}
}

func TestClaimDueLoopRunAdvancesAndSkipsOverlap(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := base
	executor := &capturingLoopExecutor{started: make(chan loops.Execution, 2)}
	service := newLoopServiceForTest(store, executor)
	service.Now = func() time.Time { return now }
	loop, err := service.Create(loops.CreateLoop{
		Prompt:   "check the thing",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}

	now = base.Add(90 * time.Second)
	ok, err := service.StartDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected due run")
	}
	started := <-executor.started
	if started.Loop.LastRunID != "" {
		t.Fatalf("prompt loop should contain previous run metadata, got last_run_id=%q", started.Loop.LastRunID)
	}
	run := started.Run
	if run.Status != loops.RunStatusStarting {
		t.Fatalf("run status = %q", run.Status)
	}
	loaded, err := store.LoadLoop(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastRunID != run.ID {
		t.Fatalf("stored last_run_id = %q, want %q", loaded.LastRunID, run.ID)
	}
	if got, want := loaded.NextRunAt, base.Add(2*time.Minute); !got.Equal(want) {
		t.Fatalf("next_run_at = %s, want %s", got, want)
	}

	now = base.Add(150 * time.Second)
	ok, err = service.StartDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("overlapping active run should be skipped, not started")
	}
	runs, err := store.ListRuns(loop.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Status != loops.RunStatusSkipped || runs[1].Status != loops.RunStatusStarting {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestStartDueAllDrainsCurrentlyDueLoops(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := base
	executor := &capturingLoopExecutor{started: make(chan loops.Execution, 2)}
	service := newLoopServiceForTest(store, executor)
	service.Now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if _, err := service.Create(loops.CreateLoop{
			Name:     "Due loop",
			Prompt:   "check the thing",
			Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	now = base.Add(90 * time.Second)
	started, err := service.StartDueAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started != 2 {
		t.Fatalf("started = %d, want 2", started)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-executor.started:
		case <-time.After(time.Second):
			t.Fatal("due loop was not dispatched")
		}
	}

	started, err = service.StartDueAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started != 0 {
		t.Fatalf("second drain started %d loops, want 0", started)
	}
}

func TestAdvanceMissedResetsStaleActiveRuns(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := base
	executor := &capturingLoopExecutor{started: make(chan loops.Execution, 2)}
	service := newLoopServiceForTest(store, executor)
	service.Now = func() time.Time { return now }
	loop, err := service.Create(loops.CreateLoop{
		Prompt:   "check the thing",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(time.Minute)
	ok, err := service.StartDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected due run")
	}
	run := (<-executor.started).Run

	now = base.Add(5 * time.Minute)
	if err := service.AdvanceMissed(); err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListRuns(loop.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != loops.RunStatusError {
		t.Fatalf("runs = %#v", runs)
	}
	now = base.Add(6 * time.Minute)
	ok, err = service.StartDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("stale run should not block future due run")
	}
}

func TestDeleteLoopSoftDeletesAndKeepsRuns(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	now := base
	executor := &capturingLoopExecutor{started: make(chan loops.Execution, 1)}
	service := newLoopServiceForTest(store, executor)
	service.Now = func() time.Time { return now }
	loop, err := service.Create(loops.CreateLoop{
		Prompt:   "check the thing",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(time.Minute)
	ok, err := service.StartDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected due run")
	}
	run := (<-executor.started).Run

	if err := service.Delete(loop.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListLoops()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		if item.ID == loop.ID {
			t.Fatalf("deleted loop should be hidden from list: %#v", listed)
		}
	}
	loaded, err := store.LoadLoop(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != loops.StatusDeleted || !loaded.NextRunAt.IsZero() {
		t.Fatalf("deleted loop = %#v", loaded)
	}
	runs, err := store.ListRuns(loop.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs after delete = %#v", runs)
	}
	now = base.Add(2 * time.Minute)
	if _, err := service.RunNow(context.Background(), loop.ID); err == nil {
		t.Fatal("manual run on deleted loop should fail")
	}
}

type capturingLoopExecutor struct {
	started chan loops.Execution
}

func (e *capturingLoopExecutor) StartLoopRun(_ context.Context, execution loops.Execution) {
	e.started <- execution
}

func newLoopServiceForTest(store *Store, executor loops.Executor) *loops.Service {
	return loops.NewService(store, executor, nil, loops.WithMemoryPaths(loops.NewMemoryPaths(filepath.Join(store.RootDir(), "automations"))))
}

func TestLoopRunSessionsAreHiddenFromDefaultSessionList(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.CreateSession(storage.CreateSession{Slug: "visible"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := store.CreateSession(storage.CreateSession{
		Slug:       "hidden-loop-run",
		SourceType: storage.SourceLoopRun,
		SourceID:   "run-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := store.ListSessions(storage.SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Slug != "visible" {
		t.Fatalf("default sessions = %#v", sessions)
	}
	sessions, err = store.ListSessions(storage.SessionFilter{SourceType: storage.SourceLoopRun})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != hidden.ID {
		t.Fatalf("loop run sessions = %#v", sessions)
	}
	loaded, err := store.LoadSession(hidden.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceID != "run-1" {
		t.Fatalf("loaded source id = %q", loaded.SourceID)
	}
}

func TestLoopMemoryPathDefaultsAreStableAndUnique(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := newLoopServiceForTest(store, nil)
	loop, err := service.Create(loops.CreateLoop{
		Name:     "Weekly Status Update Draft",
		Prompt:   "draft the status update",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * 1", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(store.RootDir(), "automations", "weekly-status-update-draft", "memory.md")
	if loop.MemoryPath != want {
		t.Fatalf("memory path = %q, want %q", loop.MemoryPath, want)
	}
	if _, err := os.Stat(filepath.Join(store.RootDir(), "automations")); !os.IsNotExist(err) {
		t.Fatalf("create should not create automations dir: %v", err)
	}
	if err := service.MemoryPaths.EnsureDir(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(store.RootDir(), "automations")); err != nil || !info.IsDir() {
		t.Fatalf("automations dir was not created: info=%v err=%v", info, err)
	}

	duplicate, err := service.Create(loops.CreateLoop{
		Name:     "Weekly Status Update Draft",
		Prompt:   "draft another status update",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * 1", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.MemoryPath == loop.MemoryPath {
		t.Fatalf("duplicate memory path reused %q", duplicate.MemoryPath)
	}
	if !strings.Contains(duplicate.MemoryPath, "weekly-status-update-draft-") {
		t.Fatalf("duplicate memory path did not include id suffix: %q", duplicate.MemoryPath)
	}

	originalPath := loop.MemoryPath
	renamed := "Renamed Loop"
	updated, err := service.Update(loop.ID, loops.UpdateLoop{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MemoryPath != originalPath {
		t.Fatalf("rename changed memory path from %q to %q", originalPath, updated.MemoryPath)
	}
}

func TestEnsureMemoryPathsBackfillsExistingLoops(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	legacy := loops.Loop{
		ID:        store.NewLoopID(),
		Name:      "Legacy Loop",
		Prompt:    "keep going",
		Schedule:  loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:    loops.StatusActive,
		Runtime:   loops.RuntimeACP,
		ACPAgent:  "codex",
		NextRunAt: now.Add(time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.SaveLoop(legacy); err != nil {
		t.Fatal(err)
	}

	service := newLoopServiceForTest(store, nil)
	if err := service.EnsureMemoryPaths(); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadLoop(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(store.RootDir(), "automations", "legacy-loop", "memory.md")
	if loaded.MemoryPath != want {
		t.Fatalf("backfilled memory path = %q, want %q", loaded.MemoryPath, want)
	}
}

func TestLoopReasoningEffortAndDirectoryRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	service := newLoopServiceForTest(store, nil)
	service.Now = func() time.Time { return now }
	loop, err := service.Create(loops.CreateLoop{
		Prompt:          "review the repo",
		Schedule:        loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Runtime:         loops.RuntimeACP,
		ACPAgent:        "codex",
		ReasoningEffort: "High",
		Directory:       " sub/dir ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if loop.ReasoningEffort != "high" || loop.Directory != "sub/dir" {
		t.Fatalf("create normalized = %q / %q", loop.ReasoningEffort, loop.Directory)
	}

	loaded, err := store.LoadLoop(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReasoningEffort != "high" || loaded.Directory != "sub/dir" {
		t.Fatalf("loaded = %q / %q", loaded.ReasoningEffort, loaded.Directory)
	}

	effort, dir := "medium", "other"
	updated, err := service.Update(loop.ID, loops.UpdateLoop{ReasoningEffort: &effort, Directory: &dir})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReasoningEffort != "medium" || updated.Directory != "other" {
		t.Fatalf("updated = %q / %q", updated.ReasoningEffort, updated.Directory)
	}

	if _, err := service.Create(loops.CreateLoop{
		Prompt:          "bad effort",
		Schedule:        loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		ReasoningEffort: "extreme",
	}); err == nil {
		t.Fatal("expected invalid reasoning effort to error")
	}
}
