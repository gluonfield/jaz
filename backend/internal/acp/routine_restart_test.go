package acp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/sessionrecovery"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// jazProcess is one run of the Jaz server over a lasting store: the real ACP
// manager with the fake agent, the server's queue and turn bookkeeping, and the
// loops service.
type jazProcess struct {
	t       *testing.T
	ctx     context.Context
	store   *sqlitestore.Store
	root    string
	manager *acp.Manager
	server  *server.Server
	loops   *loops.Service
	bots    *bots.Service
	locks   *sessionlock.Locks
	events  *sessionevents.Bus
}

func startJaz(t *testing.T, ctx context.Context, store *sqlitestore.Store, root string, env map[string]string) *jazProcess {
	t.Helper()
	env["JAZ_FAKE_ACP_RELEASE_FILE"] = filepath.Join(root, "release")
	env["JAZ_FAKE_ACP_REQUEST_LOG"] = filepath.Join(root, "requests.jsonl")
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, env, "", "")
	t.Cleanup(manager.Close)
	locks := sessionlock.New()
	events := sessionevents.New()
	handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: events}
	service := loops.NewService(store, server.NewLoopRunner(handler), log.New(io.Discard))
	handler.Loops = service
	manager.TurnFinished = handler.HandleACPTurnFinished
	botService := bots.NewService(store, root, manager, handler, handler, service, events, log.New(io.Discard))
	return &jazProcess{t: t, ctx: ctx, store: store, root: root, manager: manager, server: handler, loops: service, bots: botService, locks: locks, events: events}
}

// restart stops this process and starts the next one the way Jaz starts:
// interrupted turns resume, idle queues drain, then open routine runs settle.
// env adds to the next agent's environment.
func (p *jazProcess) restart(env map[string]string) *jazProcess {
	p.t.Helper()
	p.manager.Close()
	if env == nil {
		env = map[string]string{}
	}
	env["JAZ_FAKE_ACP_RESUME"] = "1"
	next := startJaz(p.t, p.ctx, p.store, p.root, env)
	next.recover()
	return next
}

func (p *jazProcess) recover() {
	p.t.Helper()
	if err := sessionrecovery.Resume(p.ctx, p.store, p.server, p.locks, p.events, log.New(io.Discard)); err != nil {
		p.t.Fatal(err)
	}
	if err := p.server.ResumeQueuedTurns(); err != nil {
		p.t.Fatal(err)
	}
	if err := p.server.ResumeLoopRuns(p.ctx); err != nil {
		p.t.Fatal(err)
	}
}

func (p *jazProcess) bot(name string) storage.Session {
	p.t.Helper()
	bot, err := p.manager.CreateSession(p.ctx, acp.SpawnRequest{Slug: name, Title: name, SourceType: storage.SourceBot, Directory: p.root})
	if err != nil {
		p.t.Fatal(err)
	}
	return bot
}

func (p *jazProcess) routine(input loops.CreateLoop) loops.Loop {
	p.t.Helper()
	if input.Schedule.Kind == "" && input.Trigger == nil {
		input.Schedule = loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"}
	}
	input.ACPAgent = acp.AgentCodex
	loop, err := p.loops.Create(input)
	if err != nil {
		p.t.Fatal(err)
	}
	return loop
}

// busy starts a user turn in thread that works until the test releases it.
func (p *jazProcess) busy(thread string) {
	p.t.Helper()
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: thread, Message: "answer the user until released", Completion: acp.CompletionAsync}); err != nil {
		p.t.Fatal(err)
	}
	waitForBlockingTool(p.t, p.ctx, p.manager, thread)
}

func (p *jazProcess) release() {
	p.t.Helper()
	if err := os.WriteFile(filepath.Join(p.root, "release"), nil, 0o600); err != nil {
		p.t.Fatal(err)
	}
}

func (p *jazProcess) run(id string) loops.Run {
	p.t.Helper()
	run, err := p.store.LoadRun(id)
	if err != nil {
		p.t.Fatal(err)
	}
	return run
}

func (p *jazProcess) runs(loopID string) []loops.Run {
	p.t.Helper()
	runs, err := p.loops.Runs(loopID, 50)
	if err != nil {
		p.t.Fatal(err)
	}
	return runs
}

func (p *jazProcess) waitRun(id, status string) loops.Run {
	p.t.Helper()
	var run loops.Run
	p.eventually("run "+id+" "+status, func() bool {
		run = p.run(id)
		if run.Status != status && !run.FinishedAt.IsZero() {
			p.t.Fatalf("run closed as %s (%s), want %s", run.Status, run.Error, status)
		}
		return run.Status == status
	})
	return run
}

func (p *jazProcess) eventually(what string, ready func() bool) {
	p.t.Helper()
	for !ready() {
		select {
		case <-p.ctx.Done():
			p.t.Fatalf("timed out waiting for %s", what)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// prompts is every prompt the agent was sent, in order.
func (p *jazProcess) prompts() []string {
	p.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.root, "requests.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		p.t.Fatal(err)
	}
	var prompts []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `"method":"session/prompt"`) {
			prompts = append(prompts, line)
		}
	}
	return prompts
}

func (p *jazProcess) promptsContaining(text string) int {
	count := 0
	for _, prompt := range p.prompts() {
		if strings.Contains(prompt, text) {
			count++
		}
	}
	return count
}

func (p *jazProcess) queued(thread string) []storage.QueuedMessage {
	p.t.Helper()
	session, err := p.store.LoadSession(thread)
	if err != nil {
		p.t.Fatal(err)
	}
	return session.QueuedMessages
}

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newTestStore(t *testing.T) (*sqlitestore.Store, string) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, root
}

func TestRoutineDueWhileItsBotIsBusyWaitsPendingAndRunsNext(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"}})
	jaz.busy(bot.ID)

	slot := routine.NextRunAt
	jaz.loops.Now = func() time.Time { return slot.Add(time.Second) }
	if ok, err := jaz.loops.StartDue(jaz.ctx); err != nil || !ok {
		t.Fatalf("due routine did not fire: %v, %v", ok, err)
	}
	first := jaz.runs(routine.ID)[0]
	jaz.eventually("the run queued in its bot's thread", func() bool { return len(jaz.queued(bot.ID)) == 1 })
	if run := jaz.run(first.ID); run.Status != loops.RunStatusStarting || !run.StartedAt.IsZero() {
		t.Fatalf("a run waiting behind its bot's turn = %+v, want pending", run)
	}

	jaz.release()
	jaz.waitRun(first.ID, loops.RunStatusOK)
	prompts := jaz.prompts()
	if len(prompts) != 2 || !strings.Contains(prompts[0], "answer the user") || !strings.Contains(prompts[1], "[routine] Inbox") {
		t.Fatalf("prompts = %q, want the routine after the busy turn", prompts)
	}
	jaz.loops.Now = func() time.Time { return slot.Add(time.Minute + time.Second) }
	if ok, err := jaz.loops.StartDue(jaz.ctx); err != nil || !ok {
		t.Fatalf("the next fire did not start: %v, %v", ok, err)
	}
	next := jaz.runs(routine.ID)[0]
	if next.ID == first.ID || next.Status == loops.RunStatusSkipped {
		t.Fatalf("next fire = %+v", next)
	}
	jaz.waitRun(next.ID, loops.RunStatusOK)
}

func TestUserMessageQueuedBehindABusyBotRunsBeforeAnEarlierRoutine(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain})
	jaz.busy(bot.ID)

	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the run queued", func() bool { return len(jaz.queued(bot.ID)) == 1 })
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/queue", strings.NewReader(`{"op":"append","message":{"text":"and the user's follow-up"}}`))
	res := httptest.NewRecorder()
	jaz.server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("queue status = %d, body = %s", res.Code, res.Body.String())
	}

	jaz.release()
	jaz.waitRun(run.ID, loops.RunStatusOK)
	prompts := jaz.prompts()
	if len(prompts) != 3 || !strings.Contains(prompts[1], "the user's follow-up") || !strings.Contains(prompts[2], "[routine] Inbox") {
		t.Fatalf("prompts = %q, want the user's message before the routine", prompts)
	}
}

func TestRestartKeepsARoutineRunWaitingInItsBotQueue(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain})
	jaz.busy(bot.ID)
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the run queued", func() bool { return len(jaz.queued(bot.ID)) == 1 })

	jaz = jaz.restart(nil)
	jaz.waitRun(run.ID, loops.RunStatusOK)
	if got := jaz.promptsContaining("[routine] Inbox"); got != 1 {
		t.Fatalf("the routine ran %d times, want once", got)
	}
	prompts := jaz.prompts()
	if !strings.Contains(prompts[len(prompts)-2], "Continue from where you left off") {
		t.Fatalf("prompts = %q, want the resumed user turn before the routine", prompts)
	}
	if runs := jaz.runs(routine.ID); len(runs) != 1 {
		t.Fatalf("runs = %+v, want the one queued run", runs)
	}
}

func TestRestartMidRoutineTurnResumesItAndClosesTheRunOnce(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox until released", BotID: bot.ID, Thread: loops.ThreadMain})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.waitRun(run.ID, loops.RunStatusRunning)
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)

	jaz = jaz.restart(nil)
	finished := jaz.waitRun(run.ID, loops.RunStatusOK)
	if finished.Error != "" || jaz.promptsContaining("Continue from where you left off") != 1 {
		t.Fatalf("resumed run = %+v, prompts = %q", finished, jaz.prompts())
	}
}

func TestRestartBeforeTurnBookkeepingResumesTheRoutineTurn(t *testing.T) {
	store, root := newTestStore(t)
	ctx := testContext(t)
	jaz := startJaz(t, ctx, store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain})
	// Jaz is killed as the provider ends the turn, before its bookkeeping runs.
	jaz.manager.TurnFinished = nil
	run, err := jaz.loops.RunNow(ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the provider ending the turn", func() bool {
		job, err := jaz.manager.Status(bot.ID)
		return err == nil && job.State == acp.StateIdle && jaz.promptsContaining("[routine] Inbox") == 1
	})
	jaz.manager.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	jaz = startJaz(t, ctx, store, root, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"})
	jaz.recover()
	jaz.waitRun(run.ID, loops.RunStatusOK)
	if got := jaz.promptsContaining("Continue from where you left off"); got != 1 {
		t.Fatalf("resumed turns = %d, prompts = %q", got, jaz.prompts())
	}
}

func TestRestartMidChildRunResumesTheChildAndClosesItsRun(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	routine := jaz.routine(loops.CreateLoop{Name: "Report", Prompt: "write the report until released"})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := jaz.waitRun(run.ID, loops.RunStatusRunning).ThreadID
	waitForBlockingTool(t, jaz.ctx, jaz.manager, child)

	jaz = jaz.restart(nil)
	jaz.waitRun(run.ID, loops.RunStatusOK)
	if session, err := store.LoadSession(child); err != nil || session.Status != storage.StatusIdle {
		t.Fatalf("child after its resumed turn = %+v, %v", session, err)
	}
}

func TestARoutineQueuedBehindAFailingTurnStillRuns(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain})
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "crash transport until released", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the run queued", func() bool { return len(jaz.queued(bot.ID)) == 1 })

	jaz.release()
	jaz.waitRun(run.ID, loops.RunStatusOK)
	if session, err := store.LoadSession(bot.ID); err != nil || session.Status != storage.StatusIdle {
		t.Fatalf("bot = %+v, %v; want the routine's turn to have run after the failed one", session, err)
	}
}
