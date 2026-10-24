package acp_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// startingJaz runs Jaz with an agent that holds its startup while a file
// named hold sits in its directory.
func startingJaz(t *testing.T, env map[string]string) *jazProcess {
	t.Helper()
	store, root := newTestStore(t)
	env["JAZ_FAKE_ACP_START_HOLD"] = "hold"
	env["JAZ_FAKE_ACP_START_LOG"] = filepath.Join(root, "starts")
	return startJaz(t, testContext(t), store, root, env)
}

// held is a directory whose agent holds its startup until released.
func held(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	hold := filepath.Join(dir, "hold")
	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, func() {
		if err := os.Remove(hold); err != nil {
			t.Fatal(err)
		}
	}
}

// reopen stops this process and starts the next one over the same files, as
// a restart does: threads left running come back interrupted.
func (p *jazProcess) reopen() *jazProcess {
	p.t.Helper()
	p.manager.Close()
	if err := p.store.Close(); err != nil {
		p.t.Fatal(err)
	}
	store, err := sqlitestore.New(p.root)
	if err != nil {
		p.t.Fatal(err)
	}
	p.t.Cleanup(func() { _ = store.Close() })
	next := startJaz(p.t, p.ctx, store, p.root, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"})
	next.recover()
	return next
}

func (p *jazProcess) starts() int {
	p.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.root, "starts"))
	if err != nil && !os.IsNotExist(err) {
		p.t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

// told counts the messages with text that reached bot: given to its agent or
// waiting in its queue.
func (p *jazProcess) told(bot, text string) int {
	told := p.promptsContaining(text)
	for _, message := range p.queued(bot) {
		if strings.Contains(message.Text, text) {
			told++
		}
	}
	return told
}

func (p *jazProcess) threadsOf(bot string) []storage.Session {
	p.t.Helper()
	sessions, err := p.store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		p.t.Fatal(err)
	}
	var threads []storage.Session
	for _, session := range sessions {
		if session.ParentID == bot {
			threads = append(threads, session)
		}
	}
	return threads
}

func TestCreateThreadReturnsBeforeItsAgentIsUpAndOneSlowStartHoldsUpNoOther(t *testing.T) {
	jaz := startingJaz(t, map[string]string{})
	bot := jaz.newBot("Gimli")
	tools := acp.NewMCPTools(jaz.manager)
	slow, release := held(t)

	_, waiting, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Price the part", Directory: slow, Commitment: owe(t, jaz.store, bot.ID)})
	if err != nil || waiting.State != acp.StateStarting {
		t.Fatalf("create_thread on a held agent = %+v, %v; want it back while the agent starts", waiting, err)
	}
	_, quick, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Check the stock", Directory: t.TempDir(), Commitment: owe(t, jaz.store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	done, err := jaz.manager.WaitThreads(jaz.ctx, []string{quick.ThreadID}, 10*time.Second)
	if err != nil || len(done.Threads) != 1 || done.Threads[0].State != acp.StateIdle || done.Threads[0].Assistant == "" {
		t.Fatalf("thread beside a held one = %+v, %v; want it finished", done, err)
	}
	if status, err := jaz.manager.Status(waiting.ThreadID); err != nil || status.State != acp.StateStarting {
		t.Fatalf("held thread = %+v, %v; want it starting", status, err)
	}

	release()
	done, err = jaz.manager.WaitThreads(jaz.ctx, []string{waiting.ThreadID}, 10*time.Second)
	if err != nil || len(done.Threads) != 1 || done.Threads[0].State != acp.StateIdle || done.Threads[0].Assistant == "" {
		t.Fatalf("released thread = %+v, %v; want it finished", done, err)
	}
	if jaz.promptsContaining("Price the part") != 1 || jaz.promptsContaining("Check the stock") != 1 {
		t.Fatalf("prompts = %q; want each sent once", jaz.prompts())
	}
}

func TestShutdownRefusesNewThreadsAndARestartStartsAClaimedOneOnce(t *testing.T) {
	jaz := startingJaz(t, map[string]string{})
	bot := jaz.newBot("Gimli")
	sendAndWait(t, jaz.ctx, jaz.manager, bot.ID, "Dispatch work")
	owed := owe(t, jaz.store, bot.ID)
	dir, release := held(t)
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "brochure", Prompt: "Draft the brochure until released", Directory: dir, Commitment: owed})
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the thread's agent starting", func() bool { return jaz.starts() == 2 })

	// Jaz shuts down while the agent is on its way up.
	jaz.manager.Close()
	if _, _, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Draft the flyer", Directory: t.TempDir(), Commitment: owed}); !errors.Is(err, acp.ErrClosed) {
		t.Fatalf("create_thread during shutdown: %v; want it refused", err)
	}
	if threads := jaz.threadsOf(bot.ID); len(threads) != 1 {
		t.Fatalf("threads = %d, want only the one created before shutdown", len(threads))
	}
	release()
	if jaz.promptsContaining("Draft the brochure") != 0 {
		t.Fatal("an agent that came up after shutdown began was given the thread's prompt")
	}
	for _, job := range jaz.manager.List() {
		if job.ID == thread.ThreadID {
			t.Fatal("an agent that came up after shutdown began was kept after Close")
		}
	}

	// The next Jaz starts it once, and a restart mid-work leaves it one turn.
	jaz = jaz.reopen()
	jaz.eventually("the thread working", func() bool { return jaz.promptsContaining("Draft the brochure") == 1 })
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ThreadID)
	jaz = jaz.reopen()
	jaz.release()
	jaz.eventually("the result reaching the bot's agent", func() bool { return jaz.promptsContaining("ACP session brochure") == 1 })
	jaz.idle(thread.ThreadID)
	jaz.idle(bot.ID)
	if jaz.promptsContaining("Draft the brochure") != 1 || jaz.promptsContaining("ACP session brochure") != 1 {
		t.Fatalf("prompts = %q; want the task and its result sent once each", jaz.prompts())
	}
	messages, err := jaz.store.LoadMessageRecords(thread.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	asked := 0
	for _, message := range messages {
		if strings.Contains(message.Content, "Draft the brochure") && !message.FromUser {
			asked++
		}
	}
	if asked != 1 {
		t.Fatalf("the thread shows the bot's task %d times, want once", asked)
	}
	if threads := jaz.commitment(owed).Threads; len(threads) != 1 {
		t.Fatalf("commitment threads = %+v, want the one thread", threads)
	}
}

func TestAThreadWhoseAgentCannotStartTellsItsBot(t *testing.T) {
	jaz := startingJaz(t, codexPlan("gpt-6-sol,gpt-6-luna"))
	bot := jaz.newBot("Gimli")
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "audit", Prompt: "Audit the books", Model: "gpt-6-astra", Directory: t.TempDir(), Commitment: owe(t, jaz.store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the failure reaching the bot", func() bool {
		for _, prompt := range jaz.prompts() {
			if strings.Contains(prompt, "ACP session audit") && strings.Contains(prompt, "failed") && strings.Contains(prompt, "not advertised") {
				return true
			}
		}
		return false
	})
	worker, err := jaz.store.LoadSession(thread.ThreadID)
	if err != nil || worker.Status != storage.StatusError || !strings.Contains(worker.Error, "not advertised") {
		t.Fatalf("thread = %+v, %v; want it failed with why", worker, err)
	}
	if jaz.promptsContaining("Audit the books") != 0 {
		t.Fatal("the thread ran its task on a model its agent does not offer")
	}
	// A failed worker triggers recovery without asking the user to restart it.
	jaz.settles(bot.ID, 1, 0)
}

func TestAMessageSentWhileAThreadStartsLeavesItsFirstPromptToRunFirst(t *testing.T) {
	jaz := startingJaz(t, map[string]string{})
	bot := jaz.newBot("Gimli")
	tools := acp.NewMCPTools(jaz.manager)
	dir, release := held(t)
	_, thread, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Price the part", Directory: dir, Commitment: owe(t, jaz.store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the thread's agent starting", func() bool { return jaz.starts() == 1 })
	sent := make(chan error, 1)
	go func() {
		_, _, err := tools.Send(jaz.ctx, callAs(bot.ID), acp.MCPSendInput{ThreadID: thread.ThreadID, Prompt: "Also check the stock"})
		sent <- err
	}()
	time.Sleep(100 * time.Millisecond)
	release()
	if err := <-sent; !errors.Is(err, acp.ErrSteeringUnsupported) {
		t.Fatalf("message sent while the agent started: %v; want unsupported steering refused", err)
	}
	done, err := jaz.manager.WaitThreads(jaz.ctx, []string{thread.ThreadID}, 10*time.Second)
	if err != nil || len(done.Threads) != 1 || done.Threads[0].State != acp.StateIdle || done.Threads[0].Assistant == "" {
		t.Fatalf("thread = %+v, %v; want its first prompt run", done, err)
	}
	if jaz.promptsContaining("Price the part") != 1 || jaz.promptsContaining("Also check the stock") != 0 {
		t.Fatalf("prompts = %q; want only the first prompt", jaz.prompts())
	}
	if jaz.starts() != 1 {
		t.Fatalf("agents started = %d, want one for the thread", jaz.starts())
	}
}

func TestShutdownWhileAThreadPreparesItsFirstTurnLeavesTheTurnForTheRestart(t *testing.T) {
	jaz := startingJaz(t, map[string]string{"JAZ_FAKE_ACP_PLAN_HOLD": "hold-plan"})
	bot := jaz.newBot("Gimli")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hold-plan"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Plan the launch", Plan: true, Directory: dir, Commitment: owe(t, jaz.store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the first turn switching to plan mode", func() bool {
		raw, _ := os.ReadFile(filepath.Join(jaz.root, "requests.jsonl"))
		return strings.Contains(string(raw), `"modeId":"plan"`)
	})

	// Jaz shuts down while the agent switches mode for the first turn.
	jaz.manager.Close()
	if err := os.Remove(filepath.Join(dir, "hold-plan")); err != nil {
		t.Fatal(err)
	}
	jaz = jaz.reopen()
	jaz.idle(thread.ThreadID)
	if jaz.promptsContaining("Plan the launch") != 1 {
		t.Fatalf("prompts = %q; want the first turn run once after the restart", jaz.prompts())
	}
}

// diesAtClaim is a store whose Jaz dies as it claims a queued turn.
type diesAtClaim struct{ *sqlitestore.Store }

func (s diesAtClaim) ClaimQueuedTurn(storage.Session, storage.QueuedMessage) error {
	_ = s.Store.Close()
	return errors.New("jaz died")
}

func TestJazDyingAsItClaimsANewThreadsPromptLeavesThePromptForTheRestart(t *testing.T) {
	store, root := newTestStore(t)
	bot, err := store.CreateSession(storage.CreateSession{Slug: "gimli", SourceType: storage.SourceBot, RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: acp.AgentCodex, Cwd: root}})
	if err != nil {
		t.Fatal(err)
	}
	owed := owe(t, store, bot.ID)
	dying := newFakeNamedAgentManagerWithOptions(t, diesAtClaim{store}, root, acp.AgentCodex, map[string]string{}, "", "")
	if _, _, err := acp.NewMCPTools(dying).Create(testContext(t), callAs(bot.ID), acp.MCPCreateInput{Slug: "stock", Prompt: "Count the stock", Directory: t.TempDir(), Commitment: owed}); err == nil {
		t.Fatal("create_thread succeeded although Jaz died")
	}
	dying.Close()

	reopened, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	jaz := startJaz(t, testContext(t), reopened, root, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"})
	jaz.recover()
	jaz.eventually("the result reaching the bot", func() bool { return jaz.told(bot.ID, "ACP session stock") == 1 })
	if jaz.promptsContaining(`"text":"Count the stock"`) != 1 {
		t.Fatalf("prompts = %q; want the thread's first prompt run once", jaz.prompts())
	}
	if threads := jaz.commitment(owed).Threads; len(threads) != 1 {
		t.Fatalf("commitment threads = %+v, want the one thread", threads)
	}
}
