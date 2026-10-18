package bots

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestABotsLoopToolsSeeOnlyItsOwnRoutines(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	thread := func(in storage.CreateSession) string {
		session, err := store.CreateSession(in)
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}
	bot := func(name string) string {
		id := thread(storage.CreateSession{Title: name, SourceType: storage.SourceBot})
		if err := store.SaveBot(storage.BotRecord{ThreadID: id, Kind: KindBot}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gimli, legolas := bot("Gimli"), bot("Legolas")
	subtask := thread(storage.CreateSession{SourceType: storage.SourceBotWorker, SourceID: gimli})
	seat := thread(storage.CreateSession{SourceType: storage.SourceBotMember, SourceID: gimli})
	run := thread(storage.CreateSession{SourceType: storage.SourceLoopRun, ParentID: gimli})
	chat := thread(storage.CreateSession{Title: "Chat"})

	routines := loops.NewService(store, nil, log.New(nil))
	routine := func(owner string) string {
		loop, err := routines.Create(loops.CreateLoop{
			Prompt:   "tell a joke",
			Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 */3 * * *", Timezone: "UTC"},
			BotID:    owner,
		})
		if err != nil {
			t.Fatal(err)
		}
		return loop.ID
	}
	own, theirs, unowned := routine(gimli), routine(legolas), routine("")
	tools := loops.NewMCPTools(routines, loops.WithScope(NewService(store, "", nil, nil, nil, routines, nil, log.New(nil)).BotOf))
	from := func(threadID string) *mcp.CallToolRequest {
		return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(threadID)}}
	}
	ctx := context.Background()

	for threadID, want := range map[string][]string{gimli: {own}, subtask: {own}, seat: {own}, run: {own}, chat: {own, theirs, unowned}} {
		_, out, err := tools.List(ctx, from(threadID), loops.MCPListInput{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, loop := range out.Loops {
			got = append(got, loop.ID)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("thread %s lists %v, want %v", threadID, got, want)
		}
	}
	if _, _, err := tools.Get(ctx, from(gimli), loops.MCPIDInput{ID: own}); err != nil {
		t.Fatalf("a bot could not read its own routine: %v", err)
	}
	paused := loops.StatusPaused
	for _, id := range []string{theirs, unowned} {
		_, _, getErr := tools.Get(ctx, from(seat), loops.MCPIDInput{ID: id})
		_, _, updateErr := tools.Update(ctx, from(subtask), loops.MCPUpdateInput{ID: id, Status: &paused})
		_, _, runErr := tools.Run(ctx, from(gimli), loops.MCPIDInput{ID: id})
		_, _, deleteErr := tools.Delete(ctx, from(gimli), loops.MCPIDInput{ID: id})
		if getErr == nil || updateErr == nil || runErr == nil || deleteErr == nil {
			t.Fatalf("a bot reached routine %s it does not own: get %v, update %v, run %v, delete %v", id, getErr, updateErr, runErr, deleteErr)
		}
		if loop, err := routines.Load(id); err != nil || loop.Status != loops.StatusActive {
			t.Fatalf("routine %s after foreign calls = %+v, %v", id, loop, err)
		}
	}
}

func TestARoutineRunOfItsOwnCarriesItsBotsSavedPrompt(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Title: "Gimli", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: bot.ID, Kind: KindBot, Prompt: "Sign every report as Gimli."}); err != nil {
		t.Fatal(err)
	}
	routines := loops.NewService(store, nil, log.New(nil), loops.WithPromptExtras(RoutineInstructions(store)))
	for owner, want := range map[string]bool{bot.ID: true, "": false} {
		loop, err := routines.Create(loops.CreateLoop{Prompt: "write the report", BotID: owner, Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"}})
		if err != nil {
			t.Fatal(err)
		}
		prompt := strings.Join(routines.RuntimePromptBuilder().For(loop, loops.Run{ID: "run"}, time.Now()), "\n")
		if strings.Contains(prompt, "Sign every report as Gimli.") != want {
			t.Fatalf("routine of %q: system prompt = %q, want the bot's saved prompt %v", owner, prompt, want)
		}
	}
}
