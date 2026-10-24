package bots

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// acceptingQueue stands in for the durable turn queue, which takes every turn.
type acceptingQueue struct{}

func (acceptingQueue) QueueInternalTurn(context.Context, storage.TurnDelivery) error {
	return nil
}

func newHomeService(t *testing.T) (*Service, *sqlitestore.Store, *acp.Manager) {
	t.Helper()
	service, store, manager := newConfiguredBotService(t, t.TempDir())
	service.queue = acceptingQueue{}
	return service, store, manager
}

func homeOf(t *testing.T, store *sqlitestore.Store, id string) string {
	t.Helper()
	session, err := store.LoadSession(id)
	if err != nil || session.RuntimeRef == nil || session.RuntimeRef.Cwd == "" {
		t.Fatalf("bot %s has no home: %+v, %v", id, session.RuntimeRef, err)
	}
	return session.RuntimeRef.Cwd
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestABotsThreadsAndRoutinesAreToldToReadItsHomeFiles(t *testing.T) {
	service, store, manager := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research", Prompt: "Follow robotics research."})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := service.Create(t.Context(), CreateBot{Name: "Operations"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.CreateGroup("Launch", []string{bot.ID, peer.ID})
	if err != nil {
		t.Fatal(err)
	}
	seat, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: "seat", SourceType: storage.SourceBotMember, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: bot.ID, ThreadID: seat.ID}); err != nil {
		t.Fatal(err)
	}
	home := homeOf(t, store, bot.ID)
	prompt := func(id string) []string {
		t.Helper()
		thread, err := store.LoadSession(id)
		if err != nil {
			t.Fatal(err)
		}
		modules, err := Prompt(store, thread)
		if err != nil {
			t.Fatal(err)
		}
		return modules.Strings()
	}
	routine := func(directory string) string {
		return RoutineInstructions(store)(loops.Loop{BotID: bot.ID, Directory: directory}, loops.Run{})
	}

	write(t, filepath.Join(home, memoryFile), "The user's dog is Biscuit.\n")
	write(t, filepath.Join(home, agentsFile), "Sign reports as Research.")
	for _, id := range []string{bot.ID, seat.ID} {
		modules := prompt(id)
		if text := strings.Join(modules, "\n"); strings.Contains(text, "Biscuit") || strings.Contains(text, "Sign reports") || !strings.Contains(text, "Read it before your first answer") {
			t.Fatalf("thread %s carries its home files instead of being told to read them: %q", id, modules)
		}
	}
	if text := routine(""); !strings.Contains(text, "Follow robotics research.") || !strings.Contains(text, "read MEMORY.md in your bot's home, "+home) || strings.Contains(text, "Biscuit") {
		t.Fatalf("a routine run in the bot's home got %q, want its prompt and where its memory is", text)
	}
	if text := routine("/projects/site"); !strings.Contains(text, "read AGENTS.md and MEMORY.md in your bot's home, "+home) || strings.Contains(text, "Sign reports") {
		t.Fatalf("a routine run outside the bot's home got %q, want where its AGENTS.md and memory are", text)
	}
}

func TestABotIsToldToRereadItsFilesUntilATurnSucceedsAfterTheyChange(t *testing.T) {
	service, store, manager := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	seat, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: "seat", SourceType: storage.SourceBotMember, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	owed, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "side chat"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: "side chat", ParentID: bot.ID, Commitment: owed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DropCommitment(owed.ID, "only the files matter here", time.Now()); err != nil {
		t.Fatal(err)
	}
	home := homeOf(t, store, bot.ID)
	note := func(id string) string {
		t.Helper()
		thread, err := store.LoadSession(id)
		if err != nil {
			t.Fatal(err)
		}
		note, _ := TurnNote(store)(thread)
		return note
	}
	turn := func(id, status string) {
		t.Helper()
		// Turn starts keep milliseconds; keep edits and starts apart.
		time.Sleep(5 * time.Millisecond)
		if err := store.StartSessionTurn(id, storage.Turn{}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(id, status, "", time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	memoryNote := "Your MEMORY.md changed since your last turn; read it again before you answer.\n\n"

	agents := filepath.Join(home, agentsFile)
	write(t, agents, "# Research\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(agents, old, old); err != nil {
		t.Fatal(err)
	}
	if got := note(bot.ID); got != "" {
		t.Fatalf("a new thread was told to reread an AGENTS.md older than it: %q", got)
	}
	turn(bot.ID, storage.StatusIdle)

	write(t, filepath.Join(home, memoryFile), "The user's dog is Biscuit.\n")
	if got := note(bot.ID); got != memoryNote {
		t.Fatalf("after the bot wrote MEMORY.md its thread got %q, want %q", got, memoryNote)
	}
	if got := note(seat.ID); got != memoryNote {
		t.Fatalf("after the bot wrote MEMORY.md its group thread got %q, want %q", got, memoryNote)
	}
	if got := note(worker.ID); got != "" {
		t.Fatalf("a side chat, which gets no identity prompt, got %q", got)
	}
	turn(bot.ID, storage.StatusError)
	if got := note(bot.ID); got != memoryNote {
		t.Fatalf("a failed turn counted as having shown the change: %q", got)
	}
	turn(bot.ID, storage.StatusIdle)
	if got := note(bot.ID); got != "" {
		t.Fatalf("told again after a turn that took the note succeeded: %q", got)
	}

	files, err := service.Files(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(bot.ID, agentsFile, "# Research\n\nAlways cite sources.\n", files[0].Version); err != nil {
		t.Fatal(err)
	}
	if got := note(bot.ID); got != "Your AGENTS.md changed since your last turn; read it again before you answer.\n\n" {
		t.Fatalf("after the user edited AGENTS.md the bot got %q", got)
	}
	if got := note(seat.ID); got != "Your AGENTS.md and MEMORY.md changed since your last turn; read them again before you answer.\n\n" {
		t.Fatalf("a group thread with no successful turn yet got %q", got)
	}
}

func TestSavingAnIdentityFileRefusesAStaleVersionAndKeepsOtherFilesOut(t *testing.T) {
	service, store, _ := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	home := homeOf(t, store, bot.ID)
	files, err := service.Files(bot.ID)
	if err != nil || len(files) != 2 || files[0].Name != agentsFile || files[1].Name != memoryFile || files[1].Version != "" {
		t.Fatalf("files of a new bot = %+v, %v", files, err)
	}
	saved, err := service.SaveFile(bot.ID, memoryFile, "Biscuit is a dog.\n", "")
	if err != nil || saved.Content != "Biscuit is a dog.\n" || saved.Version == "" {
		t.Fatalf("creating MEMORY.md = %+v, %v", saved, err)
	}
	if _, err := service.SaveFile(bot.ID, memoryFile, "overwritten\n", ""); err != ErrFileChanged {
		t.Fatalf("a save over a file read before it existed = %v, want ErrFileChanged", err)
	}
	write(t, filepath.Join(home, memoryFile), "Biscuit is a dog.\nHe is four.\n")
	current, err := service.SaveFile(bot.ID, memoryFile, "my edit\n", saved.Version)
	if err != ErrFileChanged || current.Content != "Biscuit is a dog.\nHe is four.\n" {
		t.Fatalf("a save over the bot's newer write = %+v, %v; want refused with the current file", current, err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, memoryFile)); string(data) != "Biscuit is a dog.\nHe is four.\n" {
		t.Fatalf("a refused save changed the file: %q", data)
	}
	// A rewrite of the same length within the same clock tick is still a change.
	info, err := os.Stat(filepath.Join(home, memoryFile))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, memoryFile), "Biscuit is a cat.\nHe is four.\n")
	if err := os.Chtimes(filepath.Join(home, memoryFile), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(bot.ID, memoryFile, "my edit\n", current.Version); err != ErrFileChanged {
		t.Fatalf("a save over a same-sized rewrite with the same time = %v, want ErrFileChanged", err)
	}
	for _, name := range []string{"notes.md", "../AGENTS.md"} {
		if _, err := service.SaveFile(bot.ID, name, "x", ""); err == nil {
			t.Fatalf("saved %q, which is not an identity file", name)
		}
	}
}

func TestActivityListsWhatABotDidAcrossItsThreads(t *testing.T) {
	service, store, manager := newHomeService(t)
	research, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := service.Create(t.Context(), CreateBot{Name: "Operations"})
	if err != nil {
		t.Fatal(err)
	}
	commitments := map[string]int64{}
	sideChat := func(owner, title string) string {
		t.Helper()
		owed, err := store.OpenCommitment(storage.Commitment{BotID: owner, Request: title})
		if err != nil {
			t.Fatal(err)
		}
		session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: title, Title: title, ParentID: owner, Commitment: owed.ID})
		if err != nil {
			t.Fatal(err)
		}
		commitments[title] = owed.ID
		return session.ID
	}
	finished, working, archived, rerun := sideChat(research.ID, "Plan review"), sideChat(research.ID, "Queue fixes"), sideChat(research.ID, "Old idea"), sideChat(research.ID, "Rerun")
	sideChat(operations.ID, "Their chat")
	// A side chat from before commitments serves none.
	legacy, err := store.CreateSession(storage.CreateSession{Slug: "legacy", Title: "Legacy notes", ParentID: research.ID, SourceType: storage.SourceBotWorker, SourceID: research.ID})
	if err != nil {
		t.Fatal(err)
	}
	running := legacy.ID
	for _, id := range []string{finished, archived, rerun} {
		if err := store.StartSessionTurn(id, storage.Turn{}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(id, storage.StatusIdle, "", time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetArchived(archived, true); err != nil {
		t.Fatal(err)
	}
	// The bot dropped the request its archived side chat served.
	if _, err := service.Commit(research.ID, Commit{ID: commitments["Old idea"], Status: storage.CommitmentDropped, Message: "Abandoned"}); err != nil {
		t.Fatal(err)
	}
	// A thread taken up again after its commitment closed runs on its own.
	if _, err := service.Commit(research.ID, Commit{ID: commitments["Rerun"], Status: storage.CommitmentDropped, Message: "Superseded"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{working, running, rerun} {
		if err := store.StartSessionTurn(id, storage.Turn{}); err != nil {
			t.Fatal(err)
		}
	}
	started, err := store.LoadSession(running)
	if err != nil || started.Turn == nil || started.Turn.StartedAt.IsZero() {
		t.Fatalf("a started turn keeps no start time: %+v, %v", started.Turn, err)
	}
	routine, err := service.routines.Create(loops.CreateLoop{Name: "Morning brief", Prompt: "Brief me.", BotID: research.ID, Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{loops.RunStatusOK, loops.RunStatusError} {
		at := time.Now().UTC()
		if err := store.SaveRun(loops.Run{ID: store.NewRunID(), LoopID: routine.ID, ThreadID: research.ID, ScheduledFor: at, StartedAt: at, FinishedAt: at, Status: status, Error: map[bool]string{true: "Gmail sign-in expired"}[status == loops.RunStatusError], CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	// A run older than Activity looks back is left out.
	old := time.Now().Add(-40 * 24 * time.Hour).UTC()
	if err := store.SaveRun(loops.Run{ID: store.NewRunID(), LoopID: routine.ID, ThreadID: research.ID, ScheduledFor: old, StartedAt: old, FinishedAt: old, Status: loops.RunStatusOK, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	// A bot writes to another and answers in a turn another bot gave it.
	if err := service.Message(research.ID, "Operations", "Which fixes matter this week?"); err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(operations.ID, storage.Turn{Output: &storage.TurnOutput{ReplyTo: research.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := service.Say(operations.ID, "The queue fixes."); err != nil {
		t.Fatal(err)
	}
	home := homeOf(t, store, research.ID)
	write(t, filepath.Join(home, memoryFile), "Ship queue fixes first.\n")
	// Changes to AGENTS.md, the bot's and the user's, are left out.
	write(t, filepath.Join(home, agentsFile), "Cite sources.\n")
	service.NoteHomeChanges(running)
	service.NoteHomeChanges(finished)
	files, err := service.Files(research.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(research.ID, agentsFile, "Cite every source.\n", files[0].Version); err != nil {
		t.Fatal(err)
	}
	// What a side chat started is listed under it, oldest first, and what
	// that started under that, archived or not; what a thread Activity does
	// not list started is listed on its own.
	startedBy := func(parent, title string, done time.Time) string {
		t.Helper()
		session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: title, Title: title, ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.StartSessionTurn(session.ID, storage.Turn{}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(session.ID, storage.StatusIdle, "", done, nil); err != nil {
			t.Fatal(err)
		}
		return session.ID
	}
	diff := startedBy(finished, "Diff review", time.Now().Add(-time.Hour).UTC())
	design := startedBy(finished, "Design review", time.Now().Add(-2*time.Hour).UTC())
	second := startedBy(diff, "Second opinion", time.Now().Add(-30*time.Minute).UTC())
	if err := store.SetArchived(design, true); err != nil {
		t.Fatal(err)
	}
	seat, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: "research in team", SourceType: storage.SourceBotMember, SourceID: research.ID})
	if err != nil {
		t.Fatal(err)
	}
	startedBy(seat.ID, "Errand for the team", time.Now().Add(-3*time.Hour).UTC())

	view, err := service.Activity(research.ID)
	if err != nil {
		t.Fatal(err)
	}
	listed := func(parent string) []string {
		var ids []string
		for _, item := range view.Started[parent] {
			ids = append(ids, item.ThreadID)
		}
		return ids
	}
	if len(view.Started) != 2 || !slices.Equal(listed(finished), []string{design, diff}) || !slices.Equal(listed(diff), []string{second}) {
		t.Fatalf("started = %+v, want the reviews under their side chat and the second opinion under its review", view.Started)
	}
	items := view.Items
	var got []string
	for i, item := range items {
		if i > 0 && (item.Running && !items[i-1].Running || item.Running == items[i-1].Running && item.At.After(items[i-1].At)) {
			t.Fatalf("activity is not what runs first, then newest first: %+v before %+v", items[i-1], item)
		}
		line := item.Kind + ":" + item.Title + ":" + item.Text + ":" + item.Status
		if item.Running {
			line += ":running"
		}
		if item.ThreadID == running && !item.At.Equal(started.Turn.StartedAt) {
			t.Fatalf("a running side chat is dated %v, want its turn's start %v", item.At, started.Turn.StartedAt)
		}
		got = append(got, line)
	}
	slices.Sort(got)
	// Side chats that serve commitments show through them.
	want := []string{
		"file_changed:MEMORY.md::",
		"message_received:Operations:The queue fixes.:",
		"message_sent:Operations:Which fixes matter this week?:",
		"routine:Morning brief::ok",
		"routine:Morning brief:Gmail sign-in expired:error",
		"side_chat:Errand for the team::idle",
		"side_chat:Legacy notes::running:running",
		"side_chat:Rerun::running:running",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("activity =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var lines []string
	for _, item := range view.Owed {
		for _, thread := range item.Threads {
			lines = append(lines, fmt.Sprintf("#%d %s: %s %s %s", item.ID, item.Request, thread.ID, thread.Title, thread.State))
		}
	}
	if want := []string{
		fmt.Sprintf("#%d Plan review: %s Plan review finished", commitments["Plan review"], finished),
		fmt.Sprintf("#%d Queue fixes: %s Queue fixes working", commitments["Queue fixes"], working),
	}; !slices.Equal(lines, want) {
		t.Fatalf("owed =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	// What runs stays listed however much happened since it started.
	for range activityShown {
		if err := store.AppendSessionEvents(research.ID, sessionevents.Event{
			SessionID: research.ID, Type: sessionevents.TypeBotActivity, At: time.Now().UTC(),
			BotActivity: &sessionevents.BotActivityEvent{Kind: "message_sent", Label: "Operations", Text: "ping"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	busy, err := service.Activity(research.ID)
	if err != nil || len(busy.Items) != activityShown+2 || !busy.Items[0].Running || !busy.Items[1].Running {
		t.Fatalf("after %d newer items the running side chats are gone: %d items, first %+v, %v", activityShown, len(busy.Items), busy.Items[:2], err)
	}
	theirs, err := service.Activity(operations.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(theirs.Items, func(item Activity) bool {
		return item.Kind == "message_received" && item.Text == "Which fixes matter this week?"
	}) {
		t.Fatalf("the bot written to has no record of the message: %+v", theirs)
	}
}

func TestIdentityFileChangesAreRecordedOnceWithTheUsersOwnEditsNamed(t *testing.T) {
	service, store, _ := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	memory := filepath.Join(homeOf(t, store, bot.ID), memoryFile)
	kinds := func() []string {
		t.Helper()
		events, err := store.LoadBotActivity(bot.ID, time.Unix(0, 0))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, event := range events {
			out = append(out, event.BotActivity.Kind+":"+event.BotActivity.Label)
		}
		return out
	}
	write(t, memory, "old\n")
	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(memory, old, old); err != nil {
		t.Fatal(err)
	}
	service.NoteHomeChanges(bot.ID)
	if got := kinds(); len(got) != 0 {
		t.Fatalf("a change older than Activity looks back was recorded: %v", got)
	}
	write(t, memory, "one\n")
	service.NoteHomeChanges(bot.ID)
	// Turns ending together can each observe the change; it is kept once.
	info, err := os.Stat(memory)
	if err != nil {
		t.Fatal(err)
	}
	service.record(bot.ID, sessionevents.BotActivityEvent{Kind: "file_changed", Label: memoryFile}, info.ModTime())
	files, err := service.Files(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(bot.ID, memoryFile, "two\n", files[1].Version); err != nil {
		t.Fatal(err)
	}
	service.NoteHomeChanges(bot.ID)
	if got, want := kinds(), []string{"file_changed:MEMORY.md", "file_edited:MEMORY.md"}; !slices.Equal(got, want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
	time.Sleep(5 * time.Millisecond)
	write(t, memory, "three\n")
	service.NoteHomeChanges(bot.ID)
	if got := kinds(); len(got) != 3 || got[2] != "file_changed:MEMORY.md" {
		t.Fatalf("a later change was not recorded: %v", got)
	}
}
