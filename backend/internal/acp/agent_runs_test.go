package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// workState has every turn of the fake agent report its work, as the Claude
// adapter does, each thread's agent under its own session id, with env added.
func workState(env ...string) map[string]string {
	out := map[string]string{"JAZ_FAKE_ACP_WORK_STATE": "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "1"}
	for i := 0; i < len(env); i += 2 {
		out[env[i]] = env[i+1]
	}
	return out
}

// startBackgroundWork has a parent thread start a thread for work that the
// thread runs in the background (see fakeBackgroundWork), and waits until the
// thread's interim result reached the parent.
func startBackgroundWork(t *testing.T, jaz *jazProcess, prompt string) (parent, thread string) {
	t.Helper()
	lead, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "lead", Title: "Lead", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	jaz.write(lead.ID, "hello")
	spawned, err := jaz.manager.Spawn(jaz.ctx, acp.SpawnRequest{ParentID: lead.ID, Slug: "runner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: spawned.SessionID, Message: prompt, Completion: acp.CompletionAsync, ParentVisible: true, From: lead.ID}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the interim result reaching the parent", func() bool { return len(jaz.resultsFor("runner")) > 0 })
	return lead.ID, spawned.SessionID
}

// resultsFor is every result of the thread slug that reached its parent.
func (p *jazProcess) resultsFor(slug string) []string {
	var results []string
	for _, prompt := range p.prompts() {
		if strings.Contains(prompt, "ACP session "+slug) {
			results = append(results, prompt)
		}
	}
	return results
}

// twoResults waits until the thread's second result reached the parent and the
// parent handled it, and returns both.
func (p *jazProcess) twoResults(parent, thread string) []string {
	p.t.Helper()
	p.eventually("the run's result reaching the parent", func() bool { return len(p.resultsFor("runner")) >= 2 })
	p.eventually("the thread's run ending", func() bool {
		session, err := p.store.LoadSession(thread)
		return err == nil && session.Status != storage.StatusRunning
	})
	p.idle(parent)
	results := p.resultsFor("runner")
	if len(results) != 2 {
		p.t.Fatalf("the parent got %d results, want the interim one and the run's: %q", len(results), results)
	}
	return results
}

// step lets the fake agent's background work go on (see fakeBackgroundWork).
func (p *jazProcess) step(name string) {
	p.t.Helper()
	if err := os.WriteFile(filepath.Join(p.root, "release-"+name), nil, 0o600); err != nil {
		p.t.Fatal(err)
	}
}

// finished waits until the thread's background work finished.
func (p *jazProcess) finished(thread string) {
	p.t.Helper()
	p.eventually("the work finishing", func() bool {
		events, err := p.store.LoadSessionEvents(thread)
		return err == nil && slices.ContainsFunc(events, func(event sessionevents.Event) bool {
			return event.AgentTask != nil && event.AgentTask.State == "completed"
		})
	})
}

// chat starts the user's own turn in thread, which works until the test
// releases it and owes no one.
func (p *jazProcess) chat(thread string) {
	p.t.Helper()
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: thread, Message: "answer the user until released", Completion: acp.CompletionInline}); err != nil {
		p.t.Fatal(err)
	}
	waitForBlockingTool(p.t, p.ctx, p.manager, thread)
}

func (p *jazProcess) running(thread string) {
	p.t.Helper()
	p.eventually(thread+" running", func() bool {
		session, err := p.store.LoadSession(thread)
		return err == nil && session.Status == storage.StatusRunning
	})
}

func TestARunTheAgentBeginsAfterItsBackgroundWorkReportsToTheParentThatAskedOnce(t *testing.T) {
	// An agent that does not name the work its run took in is taken to report
	// work that finished between turns in the next one.
	for _, unnamed := range []string{"0", "1"} {
		t.Run("unnamed="+unnamed, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, workState("JAZ_FAKE_ACP_UNNAMED", unnamed))
			parent, thread := startBackgroundWork(t, jaz, "start background work suite")
			// The user chats with the thread meanwhile; that turn owes the parent nothing.
			jaz.write(thread, "how is it going?")

			jaz.step("suite")
			jaz.step("suite-run")
			jaz.running(thread)
			jaz.step("suite-done")
			results := jaz.twoResults(parent, thread)
			if !strings.Contains(results[1], "suite finished: all green.") || strings.Contains(results[1], "Started suite") {
				t.Fatalf("results = %q; want the run's own result after the interim one", results)
			}
			session, err := store.LoadSession(thread)
			if err != nil || session.Usage.InputTokens != 7 || session.Usage.OutputTokens != 4 {
				t.Fatalf("thread usage = %+v, %v; want the run's tokens", session.Usage, err)
			}
		})
	}
}

func TestAMessageDuringTheAgentsOwnRunReportsWithItToTheParentOnce(t *testing.T) {
	for _, test := range []struct {
		message string
		answer  string
	}{
		{"also check lint", "Took the message into the run."},
		{"check lint after the run", "Answered after the run."},
	} {
		t.Run(test.message, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, workState("JAZ_FAKE_ACP_PROMPT_QUEUEING", "1"))
			parent, thread := startBackgroundWork(t, jaz, "start background work suite")
			jaz.step("suite")
			jaz.step("suite-run")
			jaz.running(thread)
			// A message for a running thread steers its turn, as the server
			// steers a turn the user writes into.
			if _, err := jaz.manager.Steer(jaz.ctx, acp.SteerRequest{Session: thread, Message: test.message}); err != nil {
				t.Fatal(err)
			}
			if result := jaz.twoResults(parent, thread)[1]; !strings.Contains(result, "suite finished: all green.") || !strings.Contains(result, test.answer) {
				t.Fatalf("result = %q; want the run and %q", result, test.answer)
			}
		})
	}
}

func TestAMessageAfterTheWorkFinishedOwesTheParentTheAgentsReport(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	parent, thread := startBackgroundWork(t, jaz, "start background work suite")
	jaz.step("suite")
	jaz.finished(thread)
	// The user writes before the agent's run about it begins; Claude Code takes
	// the message into that run.
	jaz.write(thread, "is the suite done?")
	if result := jaz.twoResults(parent, thread)[1]; !strings.Contains(result, "suite finished: all green.") {
		t.Fatalf("result = %q; want the agent's report on the work", result)
	}
}

func TestWorkThatFinishesDuringAnotherTurnIsOwedByThatTurn(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	parent, thread := startBackgroundWork(t, jaz, "start background work suite")
	// A parent that Jaz's own agent runs hears through Done.
	var done atomic.Int32
	jaz.manager.Done = func(_ context.Context, job acp.Job) {
		if job.ID == thread {
			done.Add(1)
		}
	}
	// The user's turn runs when the work finishes; Claude Code reports the
	// work in that turn.
	jaz.chat(thread)
	jaz.step("suite")
	jaz.finished(thread)
	jaz.release()
	if result := jaz.twoResults(parent, thread)[1]; !strings.Contains(result, "suite finished: all green.") {
		t.Fatalf("result = %q; want the turn that reported the work", result)
	}
	jaz.eventually("Done for the turn", func() bool { return done.Load() == 1 })
}

// askers starts a lead thread that is busy until the test lets it go on with
// step "lead", so the results its runner thread delivers wait in its queue,
// a runner thread under it, and threads for two bots the lead answers. Each
// thread has a directory of its own.
func (p *jazProcess) askers() (lead, runner string, askers []string) {
	p.t.Helper()
	ids := map[string]string{}
	for _, slug := range []string{"lead", "x", "y", "runner"} {
		dir := filepath.Join(p.root, slug)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			p.t.Fatal(err)
		}
		req := acp.SpawnRequest{Slug: slug, Title: slug, Directory: dir}
		if slug == "runner" {
			req.ParentID = ids["lead"]
		}
		session, err := p.manager.CreateSession(p.ctx, req)
		if err != nil {
			p.t.Fatal(err)
		}
		ids[slug] = session.ID
	}
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: ids["lead"], Message: "work until step lead", Completion: acp.CompletionInline}); err != nil {
		p.t.Fatal(err)
	}
	waitForBlockingTool(p.t, p.ctx, p.manager, ids["lead"])
	return ids["lead"], ids["runner"], []string{ids["x"], ids["y"]}
}

// ask has thread start work for the bot asker, which its parent answers.
func (p *jazProcess) ask(thread, asker, text string) {
	p.t.Helper()
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: thread, Message: text, Completion: acp.CompletionAsync, ParentVisible: true, ParentOutput: &storage.TurnOutput{ReplyTo: asker}}); err != nil {
		p.t.Fatal(err)
	}
	p.idle(thread)
}

// deliveries waits until lead holds want results, and returns the bot each
// answers with its text.
func (p *jazProcess) deliveries(lead string, want int) [][2]string {
	p.t.Helper()
	p.eventually("the results reaching the lead", func() bool { return len(p.queued(lead)) >= want })
	var out [][2]string
	for _, message := range p.queued(lead) {
		if message.Output == nil {
			p.t.Fatalf("result %q answers no bot", message.Text)
		}
		out = append(out, [2]string{message.Output.ReplyTo, message.Text})
	}
	if len(out) != want {
		p.t.Fatalf("lead got %d results, want %d: %q", len(out), want, out)
	}
	return out
}

func TestWorkTwoBotsAskedForThatFinishesTogetherReportsToEach(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	lead, runner, askers := jaz.askers()
	jaz.ask(runner, askers[0], "start background work alpha")
	jaz.ask(runner, askers[1], "start background work beta")
	jaz.step("alpha")
	jaz.step("beta")
	jaz.eventually("both finishing", func() bool { return jaz.finishedTasks(runner) == 2 })
	// One run takes in both, as Claude Code batches what finished together.
	jaz.step("alpha-run")
	jaz.step("alpha-done")
	results := jaz.deliveries(lead, 4)
	got := []string{results[2][0], results[3][0]}
	slices.Sort(got)
	want := slices.Clone(askers)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("the run answered %q, want each bot once: %q", got, askers)
	}
	for _, result := range results[2:] {
		if !strings.Contains(result[1], "alpha finished: all green.") || !strings.Contains(result[1], "beta finished: all green.") {
			t.Fatalf("result = %q; want the run that reports both", result[1])
		}
	}
	jaz.step("lead")
	jaz.idle(lead)
}

func TestWorkTwoBotsAskedABotForThatFinishesTogetherIsAnsweredToEach(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	x, y, helper := jaz.newBot("X"), jaz.newBot("Y"), jaz.newBot("Helper")
	for _, ask := range []struct{ from, work string }{{x.ID, "alpha"}, {y.ID, "beta"}} {
		if err := jaz.bots.Message(ask.from, helper.ID, "start background work "+ask.work); err != nil {
			t.Fatal(err)
		}
		jaz.idle(helper.ID)
	}
	jaz.step("alpha")
	jaz.step("beta")
	jaz.eventually("both finishing", func() bool { return jaz.finishedTasks(helper.ID) == 2 })
	jaz.step("alpha-run")
	jaz.running(helper.ID)
	if err := jaz.bots.Say(helper.ID, "Alpha and beta passed."); err != nil {
		t.Fatal(err)
	}
	jaz.step("alpha-done")
	jaz.eventually("the answer reaching both bots", func() bool { return jaz.promptsContaining("Alpha and beta passed.") == 2 })
	jaz.idle(x.ID)
	jaz.idle(y.ID)
	for _, bot := range []string{x.ID, y.ID} {
		if got := jaz.answers(bot, "Alpha and beta passed."); got != 1 {
			t.Fatalf("bot %s got the answer %d times, want once", bot, got)
		}
	}
}

// answers counts the prompts thread's agent was sent that contain text.
func (p *jazProcess) answers(thread, text string) int {
	p.t.Helper()
	session, err := p.store.LoadSession(thread)
	if err != nil || session.RuntimeRef == nil {
		p.t.Fatalf("load %s: %v", thread, err)
	}
	count := 0
	for _, line := range p.prompts() {
		var prompt struct {
			Params struct {
				SessionID string `json:"sessionId"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &prompt) == nil && prompt.Params.SessionID == session.RuntimeRef.SessionID && strings.Contains(line, text) {
			count++
		}
	}
	return count
}

func TestARestartDuringARunTwoBotsWaitForResumesItForEach(t *testing.T) {
	store, root := newTestStore(t)
	// A restarted agent resumes the session id of its thread's directory.
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd"})
	lead, runner, askers := jaz.askers()
	jaz.ask(runner, askers[0], "start background work alpha")
	jaz.ask(runner, askers[1], "start background work beta")
	jaz.step("alpha")
	jaz.step("beta")
	jaz.eventually("both finishing", func() bool { return jaz.finishedTasks(runner) == 2 })
	jaz.step("alpha-run")
	jaz.running(runner)
	jaz.deliveries(lead, 2)

	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd"})
	jaz.eventually("the resumed run's results reaching the lead", func() bool {
		return jaz.promptsContaining("[result for the bot that asked]") == 4
	})
	jaz.idle(lead)
	if got := jaz.promptsContaining("[result for the bot that asked]"); got != 4 {
		t.Fatalf("lead got %d results, want the two interim ones and the resumed run's for each bot", got)
	}
}

func TestWorkATurnTookInIsNotOwedAgainByTheRunAfterIt(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	lead, runner, askers := jaz.askers()
	jaz.ask(runner, askers[0], "start background work alpha")
	jaz.ask(runner, askers[1], "start background work late")
	// The user's turn takes alpha in. Late finishes during its last model
	// call, so Claude Code reports it in a run after the turn.
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: runner, Message: "work until step chat", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, runner)
	jaz.step("alpha")
	jaz.step("late")
	jaz.eventually("both finishing", func() bool { return jaz.finishedTasks(runner) == 2 })
	jaz.step("chat")
	jaz.idle(runner)
	jaz.step("late-run")
	jaz.step("late-done")
	results := jaz.deliveries(lead, 4)
	if results[2][0] != askers[0] || !strings.Contains(results[2][1], "alpha finished: all green.") {
		t.Fatalf("third result = %q; want the turn's report to the bot that asked for alpha", results[2])
	}
	if results[3][0] != askers[1] || !strings.Contains(results[3][1], "late finished: all green.") {
		t.Fatalf("fourth result = %q; want the run's report to the bot that asked for late", results[3])
	}
	jaz.step("lead")
	jaz.idle(lead)
}

func TestWorkTheModelWasNeverToldOfIsOwedByNoLaterRun(t *testing.T) {
	for _, test := range []struct {
		name   string
		finish func(jaz *jazProcess, runner string)
	}{
		// Claude Code tells the model of work that finishes during a turn only
		// by naming it, so unnamed it waits for no run.
		{"finished during a turn", func(jaz *jazProcess, runner string) {
			if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: runner, Message: "work until step chat", Completion: acp.CompletionInline}); err != nil {
				t.Fatal(err)
			}
			waitForBlockingTool(t, jaz.ctx, jaz.manager, runner)
			jaz.step("lost")
			jaz.eventually("lost finishing", func() bool { return jaz.finishedTasks(runner) == 1 })
			jaz.step("chat")
			jaz.idle(runner)
		}},
		// A run tells the model of work that finished while none ran; a work
		// that ends without naming it shows no run ever will.
		{"finished while idle", func(jaz *jazProcess, runner string) {
			jaz.step("lost")
			jaz.eventually("lost finishing", func() bool { return jaz.finishedTasks(runner) == 1 })
			jaz.write(runner, "how is it going?")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, workState())
			lead, runner, askers := jaz.askers()
			jaz.ask(runner, askers[0], "start background work lost untold")
			jaz.ask(runner, askers[1], "start background work suite")
			test.finish(jaz, runner)
			jaz.step("suite")
			jaz.step("suite-run")
			jaz.step("suite-done")
			results := jaz.deliveries(lead, 3)
			if results[2][0] != askers[1] || !strings.Contains(results[2][1], "suite finished: all green.") {
				t.Fatalf("third result = %q; want the run's report to the bot that asked for suite only", results[2])
			}
			jaz.step("lead")
			jaz.idle(lead)
		})
	}
}

func TestARunReportingWorkAPersonAskedABotForShowsItsReplyInTheBotsChat(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	bot := jaz.newBot("Helper")
	jaz.write(bot.ID, "start background work suite")
	jaz.step("suite")
	jaz.step("suite-run")
	jaz.step("suite-done")
	jaz.eventually("the run's reply in the bot's chat", func() bool {
		return slices.ContainsFunc(jaz.notices(bot.ID), func(text string) bool { return strings.Contains(text, "suite finished: all green.") })
	})
}

func (p *jazProcess) finishedTasks(thread string) int {
	events, err := p.store.LoadSessionEvents(thread)
	if err != nil {
		return 0
	}
	done := map[string]bool{}
	for _, event := range events {
		if event.AgentTask != nil && event.AgentTask.State == "completed" {
			done[event.AgentTask.ID] = true
		}
	}
	return len(done)
}

func TestARestartDuringTheAgentsOwnRunResumesItForTheParentThatAsked(t *testing.T) {
	store, root := newTestStore(t)
	// A restarted agent resumes the one session id the fake serves, so only
	// the background work's turn reports its work.
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	parent, thread := startBackgroundWork(t, jaz, "start background work suite")
	jaz.step("suite")
	jaz.step("suite-run")
	jaz.running(thread)

	jaz = jaz.restart(nil)
	jaz.twoResults(parent, thread)
	if got := jaz.promptsContaining("Continue from where you left off"); got != 1 {
		t.Fatalf("resumed %d times, want once: %q", got, jaz.prompts())
	}
}

func TestARunThatBeginsAsTheTurnEndsReportsSeparately(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	// The run's report arrives while the turn finishes.
	var held sync.Once
	finish := jaz.manager.TurnFinished
	jaz.manager.TurnFinished = func(ctx context.Context, job acp.Job) {
		if job.Slug == "runner" {
			held.Do(func() {
				waitForFakeFile(filepath.Join(root, "release-lint-emitted"))
				time.Sleep(100 * time.Millisecond)
			})
		}
		finish(ctx, job)
	}
	parent, thread := startBackgroundWork(t, jaz, "start background work lint at once")
	results := jaz.twoResults(parent, thread)
	if strings.Contains(results[0], "lint finished") || !strings.Contains(results[1], "lint finished: all green.") {
		t.Fatalf("results = %q; want the turn's and the run's results apart", results)
	}
}

func TestABackgroundAgentsReportInTheTurnHeldForItReachesTheParentOnce(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	lead, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "lead", Title: "Lead", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	jaz.write(lead.ID, "hello")
	spawned, err := jaz.manager.Spawn(jaz.ctx, acp.SpawnRequest{ParentID: lead.ID, Slug: "runner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: spawned.SessionID, Message: "start background agent scout", Completion: acp.CompletionAsync, ParentVisible: true, From: lead.ID}); err != nil {
		t.Fatal(err)
	}
	// The turn's work ends at its result; its answer waits for the agent.
	jaz.step("scout")
	jaz.eventually("the turn's result reaching the parent", func() bool { return len(jaz.resultsFor("runner")) > 0 })
	jaz.idle(spawned.SessionID)
	jaz.idle(lead.ID)
	results := jaz.resultsFor("runner")
	if len(results) != 1 || !strings.Contains(results[0], "Launched scout") || !strings.Contains(results[0], "scout reported: all green.") {
		t.Fatalf("results = %q; want one result with the agent's report", results)
	}
}

func TestATurnTheProviderFailedGoesOnAfterItsWorkEnded(t *testing.T) {
	acp.SetProviderRetryDelays(t, time.Millisecond)
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState("JAZ_FAKE_ACP_FAILURE_FILE", filepath.Join(root, "failures")))
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	var turns []acp.Job
	var mu sync.Mutex
	finish := jaz.manager.TurnFinished
	jaz.manager.TurnFinished = func(ctx context.Context, job acp.Job) {
		if job.ID == thread.ID {
			mu.Lock()
			turns = append(turns, job)
			mu.Unlock()
		}
		finish(ctx, job)
	}
	// The agent reports the failed work's end before the failure; the
	// continuation Jaz sends then works in the same turn.
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip provider fails 1 overloaded", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(thread.ID)
	mu.Lock()
	defer mu.Unlock()
	if len(turns) != 1 || turns[0].State != acp.StateIdle || jaz.promptsContaining(continuation) != 1 {
		t.Fatalf("turns = %+v, prompts = %q; want one turn that a continuation answers", turns, jaz.prompts())
	}
}

func TestARunTheAgentBeginsThatFailsTellsTheParentThatAsked(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, workState())
	parent, thread := startBackgroundWork(t, jaz, "start background work broken")
	jaz.step("broken")
	jaz.step("broken-run")
	jaz.step("broken-done")
	if result := jaz.twoResults(parent, thread)[1]; !strings.Contains(result, "API Error: Overloaded") {
		t.Fatalf("result = %q; want the run's failure", result)
	}
}

func TestWorkOfAnAgentThatDoesNotReportItsWorkOwesNoLaterTurn(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	parent, thread := startBackgroundWork(t, jaz, "start background work suite quietly")
	jaz.step("suite")
	jaz.finished(thread)
	jaz.write(thread, "is the suite done?")
	jaz.idle(parent)
	if results := jaz.resultsFor("runner"); len(results) != 1 {
		t.Fatalf("the parent got %d results, want only the turn it asked for: %q", len(results), results)
	}
}
