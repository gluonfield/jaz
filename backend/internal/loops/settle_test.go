package loops_test

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/loops"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// dispatched records the runs a service hands its executor, which in Jaz
// starts each one's turn.
type dispatched chan loops.Run

func (d dispatched) StartLoopRun(_ context.Context, execution loops.Execution) {
	d <- execution.Run
}

func (d dispatched) none(t *testing.T, why string) {
	t.Helper()
	select {
	case run := <-d:
		t.Fatalf("run %s of %s started %s", run.ID, run.LoopID, why)
	case <-time.After(50 * time.Millisecond):
	}
}

func (d dispatched) next(t *testing.T) loops.Run {
	t.Helper()
	select {
	case run := <-d:
		return run
	case <-time.After(5 * time.Second):
		t.Fatal("no run started")
		return loops.Run{}
	}
}

// database runs statements on the store's database beside the store, to make
// it fail the way a broken database does.
func database(t *testing.T, root string) func(string) {
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return func(query string) {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
}

// waitBlocked waits until a listed routine says why it waits, in parts.
func waitBlocked(t *testing.T, service *loops.Service, parts ...string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		items, err := service.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 && !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(items[0].Blocked, part) }) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s blocked = %q, want %q", items[0].Name, items[0].Blocked, parts)
		}
	}
}

func TestRoutinesWaitUntilTheRunsJazLeftOpenAreSettled(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	started := make(dispatched, 8)
	service := loops.NewService(store, started, log.New(io.Discard))
	hourly := loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}
	// The last process began a run of each routine that no thread holds any
	// more, and left the next run of each waiting behind it.
	waiting := map[string]string{}
	var left []string
	for _, name := range []string{"Inbox", "Digest"} {
		loop, err := service.Create(loops.CreateLoop{Name: name, Prompt: "work on " + name, ACPAgent: "codex", Schedule: hourly})
		if err != nil {
			t.Fatal(err)
		}
		begun := loop.NextRunAt.Add(-time.Hour)
		runs := []loops.Run{
			{ID: store.NewRunID(), LoopID: loop.ID, Status: loops.RunStatusRunning, ScheduledFor: begun, StartedAt: begun, CreatedAt: begun},
			{ID: store.NewRunID(), LoopID: loop.ID, Status: loops.RunStatusStarting, ScheduledFor: loop.NextRunAt, CreatedAt: begun},
		}
		for _, run := range runs {
			if err := store.SaveRunAndLoop(run, nil); err != nil {
				t.Fatal(err)
			}
		}
		left = append(left, runs[0].ID)
		waiting[runs[1].ID] = name
	}
	exec := database(t, root)
	service.Now = func() time.Time { return time.Now().Add(time.Hour) }

	// Every try fails to read the runs: the scheduler gives up without
	// starting or skipping a due slot.
	loops.SetSettleDelays(t, time.Millisecond, time.Millisecond)
	exec("ALTER TABLE loop_runs RENAME TO loop_runs_lost")
	gaveUp := make(chan error, 1)
	go func() {
		gaveUp <- loops.StartScheduler(ctx, service, time.Millisecond, func(ctx context.Context) error { return service.Resume(ctx, nil) })
	}()
	select {
	case err := <-gaveUp:
		if err == nil {
			t.Fatal("the scheduler stopped without an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduler ran on with the open runs unsettled")
	}
	exec("ALTER TABLE loop_runs_lost RENAME TO loop_runs")
	waitBlocked(t, service, "scheduled runs wait until Jaz restarts")
	started.none(t, "before the open runs settled")
	items, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, loop := range items {
		if runs, err := service.Runs(loop.ID, 10); err != nil || len(runs) != 2 || runs[0].Status == loops.RunStatusSkipped {
			t.Fatalf("%s runs = %+v, %v; want only the two left open", loop.Name, runs, err)
		}
	}

	// At the next start the first try closes one run left open, then fails to
	// close the other. The second try closes it, and every waiting run starts
	// once.
	next := loops.NewService(store, started, log.New(io.Discard))
	next.Now = service.Now
	loops.SetSettleDelays(t, 500*time.Millisecond, 500*time.Millisecond)
	exec(`CREATE TRIGGER close_one BEFORE UPDATE OF status ON loop_runs
		WHEN NEW.status = 'error' AND (SELECT count(*) FROM loop_runs WHERE status = 'error') > 0
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`)
	go func() {
		_ = loops.StartScheduler(ctx, next, time.Hour, func(ctx context.Context) error { return next.Resume(ctx, nil) })
	}()
	waitBlocked(t, next, "scheduled runs wait while it tries again")
	exec("DROP TRIGGER close_one")
	for range len(waiting) {
		run := started.next(t)
		if _, ok := waiting[run.ID]; !ok {
			t.Fatalf("started %+v, want a run that waited", run)
		}
		delete(waiting, run.ID)
	}
	started.none(t, "twice")
	for _, id := range left {
		if run, err := store.LoadRun(id); err != nil || run.Status != loops.RunStatusError {
			t.Fatalf("run left open = %+v, %v; want it closed", run, err)
		}
	}
	items, err = next.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, loop := range items {
		if loop.Blocked != "" {
			t.Fatalf("%s blocked = %q after the runs settled", loop.Name, loop.Blocked)
		}
	}
}
