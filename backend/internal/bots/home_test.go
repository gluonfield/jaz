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

func assertTaskLifecycleGuidance(t *testing.T, prompt string, want int) {
	t.Helper()
	for _, guidance := range []string{
		"A worker or thread ending is not task completion",
		"Keep partial work in_progress",
		"Stop known owned outside jobs through their owner",
		"answered_question equal to its current question_seq",
		"check_at (RFC3339)",
		"depends_on=[] clear",
		"task to create_thread or send_message_to_thread",
	} {
		if got := strings.Count(prompt, guidance); got != want {
			t.Errorf("task lifecycle guidance %q appears %d times, want %d", guidance, got, want)
		}
	}
}

func TestBotChatsReceiveIdentityAndRoutineWorkersReceiveMemoryAccess(t *testing.T) {
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
		modules, err := Prompt(store, service.instructions, thread)
		if err != nil {
			t.Fatal(err)
		}
		return modules.Strings()
	}
	routine := func(directory string) string {
		return RoutineInstructions(store)(loops.Loop{BotID: bot.ID, Directory: directory}, loops.Run{})
	}

	write(t, filepath.Join(home, memoryFile), "The user's dog is Biscuit.\n")
	write(t, filepath.Join(home, instructionsFile), "Sign reports as Research.")
	for _, id := range []string{bot.ID, seat.ID} {
		modules := prompt(id)
		if text := strings.Join(modules, "\n"); strings.Contains(text, "Biscuit") || !strings.Contains(text, "Sign reports") || !strings.Contains(text, "Read it before your first answer") {
			t.Fatalf("thread %s must receive identity instructions and read memory on demand: %q", id, modules)
		}
		assertTaskLifecycleGuidance(t, strings.Join(modules, "\n"), 1)
	}
	if text := routine(""); strings.Contains(text, "Follow robotics research.") || !strings.Contains(text, "read MEMORY.md in your bot's home, "+home) || strings.Contains(text, "Biscuit") {
		t.Fatalf("a routine run in the bot's home got %q, want memory access without bot policy", text)
	}
	if text := routine("/projects/site"); !strings.Contains(text, "read MEMORY.md in your bot's home, "+home) || strings.Contains(text, "Sign reports") {
		t.Fatalf("a routine run outside the bot's home got %q, want memory access without bot identity", text)
	}
}

func TestBotWorkersReceiveTheOwnersMemoryAndLinkedWorkspacePaths(t *testing.T) {
	service, store, manager := newHomeService(t)
	project := t.TempDir()
	bot, err := service.Create(t.Context(), CreateBot{Name: "VPN", Prompt: "Maintain the VPN project.", Workspaces: []string{project}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bot.Workspaces) != 1 || bot.Workspaces[0] != project {
		t.Fatalf("created bot workspaces = %v, want %s", bot.Workspaces, project)
	}
	write(t, filepath.Join(homeOf(t, store, bot.ID), memoryFile), "VPN uses the north tunnel.")
	commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "inspect tunnel health"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ParentID: bot.ID, Slug: "inspect", Commitment: commitment.ID})
	if err != nil {
		t.Fatal(err)
	}
	modules, err := WorkerPrompt(store, worker)
	if err != nil {
		t.Fatal(err)
	}
	prompt := strings.Join(modules.Strings(), "\n")
	for _, expected := range []string{"MEMORY.md", homeOf(t, store, bot.ID), project, "AGENTS.md", "not copied"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("worker context omitted %q: %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "Maintain the VPN project.") || strings.Contains(prompt, "BOTS.md") {
		t.Fatalf("worker inherited bot coordination policy: %s", prompt)
	}
}

func TestBotWorkspaceEditsReachExistingWorkersAndValidateServerPaths(t *testing.T) {
	service, store, manager := newHomeService(t)
	first, next := t.TempDir(), t.TempDir()
	bot, err := service.Create(t.Context(), CreateBot{Name: "Projects", Workspaces: []string{first}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ParentID: bot.ID, Slug: "inspect", Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{next, next}
	updated, err := service.Update(t.Context(), bot.ID, UpdateBot{Workspaces: &paths})
	if err != nil || !slices.Equal(updated.Workspaces, []string{next}) {
		t.Fatalf("workspace edit: %+v, %v", updated.Workspaces, err)
	}
	worker.RuntimeRef.SessionID = "existing-native-session"
	note, version := TurnNote(store, service.instructions)(worker)
	if !strings.Contains(note, next) || strings.Contains(note, first) {
		t.Fatalf("worker did not receive new links: %q", note)
	}
	worker.PromptDigest = version
	if note, _ := TurnNote(store, service.instructions)(worker); note != "" {
		t.Fatalf("unchanged context repeated: %q", note)
	}
	invalid := []string{"relative-client-path"}
	if _, err := service.Update(t.Context(), bot.ID, UpdateBot{Workspaces: &invalid}); err == nil {
		t.Fatal("relative client path accepted")
	}
	missing := []string{filepath.Join(next, "missing")}
	if _, err := service.Update(t.Context(), bot.ID, UpdateBot{Workspaces: &missing}); err == nil {
		t.Fatal("missing server path accepted")
	}
	record, err := store.LoadBot(bot.ID)
	if err != nil || !slices.Equal(record.Workspaces, []string{next}) {
		t.Fatalf("failed edit changed saved links: %v, %v", record.Workspaces, err)
	}
	paths = []string{}
	if _, err := service.Update(t.Context(), bot.ID, UpdateBot{Workspaces: &paths}); err != nil {
		t.Fatal(err)
	}
	modules, err := WorkerPrompt(store, worker)
	if err != nil || strings.Contains(modules.Text(), next) {
		t.Fatalf("removed project still connected: %v, %v", modules, err)
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
		note, _ := TurnNote(store, service.instructions)(thread)
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

	agents := filepath.Join(home, instructionsFile)
	write(t, agents, "# Research\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(agents, old, old); err != nil {
		t.Fatal(err)
	}
	if got := note(bot.ID); got != "" {
		t.Fatalf("a new thread was told to reread an BOTS.md older than it: %q", got)
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
	if _, err := service.SaveFile(bot.ID, instructionsFile, "# Research\n\nAlways cite sources.\n", files[0].Version); err != nil {
		t.Fatal(err)
	}
	if got := note(bot.ID); got != "Your BOTS.md changed since your last turn; read it again before you answer.\n\n" {
		t.Fatalf("after the user edited BOTS.md the bot got %q", got)
	}
	if got := note(seat.ID); got != "Your BOTS.md and MEMORY.md changed since your last turn; read them again before you answer.\n\n" {
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
	if err != nil || len(files) != 2 || files[0].Name != instructionsFile || files[1].Name != memoryFile || files[1].Version != "" {
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
	for _, name := range []string{"notes.md", "../BOTS.md"} {
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
	// A task closed before Activity looks back is left out.
	ancient, err := store.OpenCommitment(storage.Commitment{BotID: research.ID, Request: "Ancient idea"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DropCommitment(ancient.ID, "Too old", time.Now().Add(-40*24*time.Hour)); err != nil {
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
	// A run in a thread of its own is listed; one in the bot's own thread
	// shows in its chat instead.
	run := func(at time.Time, status, failure string, own bool) {
		id := store.NewRunID()
		thread := research.ID
		if own {
			session, err := store.CreateSession(storage.CreateSession{Slug: "morning-brief", Title: "Morning brief", ParentID: research.ID, SourceType: storage.SourceLoopRun, SourceID: id})
			if err != nil {
				t.Fatal(err)
			}
			thread = session.ID
		}
		if err := store.SaveRun(loops.Run{ID: id, LoopID: routine.ID, ThreadID: thread, ScheduledFor: at, StartedAt: at, FinishedAt: at, Status: status, Error: failure, CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	run(now, loops.RunStatusOK, "", true)
	run(now, loops.RunStatusError, "Gmail sign-in expired", true)
	run(now, loops.RunStatusOK, "", false)
	// A run older than Activity looks back is left out.
	run(now.Add(-40*24*time.Hour), loops.RunStatusOK, "", true)
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
	// Changes to BOTS.md, the bot's and the user's, are left out.
	write(t, filepath.Join(home, instructionsFile), "Cite sources.\n")
	service.NoteHomeChanges(running)
	service.NoteHomeChanges(finished)
	files, err := service.Files(research.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(research.ID, instructionsFile, "Cite every source.\n", files[0].Version); err != nil {
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
		if i > 0 && item.At.After(items[i-1].At) {
			t.Fatalf("activity is not newest first: %+v before %+v", items[i-1], item)
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
	// Side chats that serve commitments show through them, open or closed,
	// unless taken up again; a change to MEMORY.md is not a thread and is
	// left out.
	want := []string{
		"routine:Morning brief::ok",
		"routine:Morning brief:Gmail sign-in expired:error",
		"side_chat:Errand for the team::idle",
		"side_chat:Legacy notes::running:running",
		"side_chat:Rerun::running:running",
		"task:Old idea::cancelled",
		"task:Rerun::cancelled",
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
	if _, err := service.Commit(research.ID, Commit{ID: commitments["Plan review"], Status: storage.CommitmentDone, Message: "Review delivered"}); err != nil {
		t.Fatal(err)
	}
	delivered, err := service.Activity(research.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(delivered.Owed, func(task Owed) bool { return task.ID == commitments["Plan review"] }) || !slices.ContainsFunc(delivered.Items, func(item Activity) bool {
		return item.Kind == "task" && item.Status == storage.CommitmentDone && item.Task.ID == commitments["Plan review"] && item.Task.Threads[0].ID == finished
	}) || slices.ContainsFunc(delivered.Items, func(item Activity) bool { return item.ThreadID == finished }) {
		t.Fatalf("delivered work did not move from tasks to history as its task: %+v", delivered)
	}
	if len(delivered.Started[finished]) != 2 || len(delivered.Started[diff]) != 1 {
		t.Fatalf("delivered work lost its nested threads: %+v", delivered.Started)
	}
	// Messages between bots stay in each bot's thread, out of Activity.
	theirs, err := service.Activity(operations.ID)
	if err != nil {
		t.Fatal(err)
	}
	received, err := store.LoadBotActivity(operations.ID, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs.Items) != 0 || !slices.ContainsFunc(received, func(event sessionevents.Event) bool {
		return event.BotActivity.Kind == "message_received" && event.BotActivity.Text == "Which fixes matter this week?"
	}) {
		t.Fatalf("Activity lists a message between bots, or the bot written to has no record of it: %+v, %+v", theirs.Items, received)
	}
}

func TestActivityListsTheNewestThirtyShownRowsAcrossDays(t *testing.T) {
	service, store, _ := newHomeService(t)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	hours := func(n int) time.Time { return now.Add(-time.Duration(n) * time.Hour) }
	finish := func(id string, at time.Time) {
		t.Helper()
		if err := store.StartSessionTurn(id, storage.Turn{StartedAt: at}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(id, storage.StatusIdle, "", at, nil); err != nil {
			t.Fatal(err)
		}
	}
	thread := func(parent, title string, task int64, done time.Time) string {
		t.Helper()
		session, err := store.CreateSession(storage.CreateSession{Slug: title, Title: title, ParentID: parent, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
		if err != nil {
			t.Fatal(err)
		}
		if task > 0 {
			if err := store.LinkCommitmentThread(bot.ID, session.ID, task); err != nil {
				t.Fatal(err)
			}
		}
		finish(session.ID, done)
		return session.ID
	}
	task := func(request string) int64 {
		t.Helper()
		owed, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: request})
		if err != nil {
			t.Fatal(err)
		}
		return owed.ID
	}
	// Rows that do not show, all newer than any that do, take no place: the
	// threads of open and closed tasks, even the oldest closed one's, what a
	// side chat started, and a routine's many runs in the bot's own thread.
	delivered := task("Delivered")
	worked := []string{thread(bot.ID, "Delivery draft", delivered, now.Add(-10*time.Minute)), thread(bot.ID, "Delivery check", delivered, now.Add(-11*time.Minute))}
	if _, err := service.Commit(bot.ID, Commit{ID: delivered, Status: storage.CommitmentDone, Message: "Delivered"}); err != nil {
		t.Fatal(err)
	}
	for h := 1; h < 69; h += 2 {
		dropped := task(fmt.Sprintf("Dropped %dh", h))
		if h == 67 {
			thread(bot.ID, "Late follow-up", dropped, now.Add(-15*time.Minute))
		}
		if err := store.DropCommitment(dropped, "Superseded", hours(h)); err != nil {
			t.Fatal(err)
		}
	}
	open := task("Still open")
	thread(bot.ID, "Open work", open, now.Add(-time.Minute))
	var chats []string
	for h := 2; h <= 20; h += 2 {
		chat := thread(bot.ID, fmt.Sprintf("Chat %dh", h), 0, hours(h))
		chats = append(chats, chat)
	}
	thread(chats[0], "Nested review", 0, now.Add(-20*time.Minute))
	// A running side chat counts by when it started, like any other row.
	for title, started := range map[string]time.Time{"Recent run": now.Add(-28*time.Hour - 30*time.Minute), "Long run": hours(72)} {
		session, err := store.CreateSession(storage.CreateSession{Slug: title, Title: title, ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.StartSessionTurn(session.ID, storage.Turn{StartedAt: started}); err != nil {
			t.Fatal(err)
		}
	}
	routine, err := service.routines.Create(loops.CreateLoop{Name: "Nightly sweep", Prompt: "Sweep.", BotID: bot.ID, Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 2 * * *", Timezone: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	run := func(at time.Time, own bool) {
		t.Helper()
		id := store.NewRunID()
		threadID := bot.ID
		if own {
			session, err := store.CreateSession(storage.CreateSession{Slug: "nightly-sweep", Title: "Nightly sweep", ParentID: bot.ID, SourceType: storage.SourceLoopRun, SourceID: id})
			if err != nil {
				t.Fatal(err)
			}
			threadID = session.ID
		}
		if err := store.SaveRun(loops.Run{ID: id, LoopID: routine.ID, ThreadID: threadID, ScheduledFor: at, StartedAt: at, FinishedAt: at, Status: loops.RunStatusOK, CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	for h := 22; h <= 30; h += 2 {
		run(hours(h), true)
	}
	for m := range 25 {
		run(now.Add(-time.Duration(m+5)*time.Minute), false)
	}

	view, err := service.Activity(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The newest 30: hours 0 to 28, then the side chat running since 28.5
	// hours ago; the task dropped 29 hours ago and the side chat running for
	// three days are cut.
	want := []string{"task:Delivered"}
	for h := 1; h <= 28; h++ {
		switch {
		case h%2 == 1:
			want = append(want, fmt.Sprintf("task:Dropped %dh", h))
		case h <= 20:
			want = append(want, fmt.Sprintf("side_chat:Chat %dh", h))
		default:
			want = append(want, "routine:Nightly sweep")
		}
	}
	want = append(want, "side_chat:Recent run")
	var got []string
	for _, item := range view.Items {
		got = append(got, item.Kind+":"+item.Title)
	}
	if !slices.Equal(got, want) || !view.Items[activityShown-1].Running {
		t.Fatalf("activity =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if threads := view.Items[0].Task.Threads; len(threads) != 2 || !slices.ContainsFunc(threads, func(thread OwedThread) bool { return thread.ID == worked[0] }) || !slices.ContainsFunc(threads, func(thread OwedThread) bool { return thread.ID == worked[1] }) {
		t.Fatalf("a listed task lost its threads: %+v", threads)
	}
	if len(view.Owed) != 1 || view.Owed[0].ID != open || len(view.Owed[0].Threads) != 1 || len(view.Started[chats[0]]) != 1 {
		t.Fatalf("the open task or what a side chat started went missing: %+v, %+v", view.Owed, view.Started)
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
