package loops_test

import (
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/loops"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// A hold stored by an earlier process holds the bots from the start, before
// this process's guard has run.
func TestHeldBotsReadTheStoredHoldFromStartup(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := loops.NewService(store, make(dispatched, 8), log.New(io.Discard))
	hourly := loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}
	for _, routine := range []loops.CreateLoop{
		{Name: "Inbox", Prompt: "check the inbox", BotID: "gimli", Schedule: hourly},
		{Name: "Monitor", Prompt: "watch the furnace", BotID: "legolas", Schedule: hourly, KeepRunning: true},
	} {
		routine.ACPAgent = "codex"
		if _, err := service.Create(routine); err != nil {
			t.Fatal(err)
		}
	}
	if held, err := service.HeldBots(); err != nil || held != nil {
		t.Fatalf("held = %v, %v before any hold, want none", held, err)
	}
	if err := store.SetAwaySince(time.Now().Add(-72 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	held, err := loops.NewService(store, make(dispatched, 8), log.New(io.Discard)).HeldBots()
	if err != nil || len(held) != 1 || !held["gimli"] {
		t.Fatalf("held = %v, %v, want only gimli, whose routine waits", held, err)
	}
}
