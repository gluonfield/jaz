package acp_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

const continuation = "Continue from where you left off"

func startFailingJaz(t *testing.T, delays ...time.Duration) *jazProcess {
	t.Helper()
	acp.SetProviderRetryDelays(t, delays...)
	store, root := newTestStore(t)
	env := failingProvider(root)
	// Codex steers a running turn natively.
	env["JAZ_FAKE_ACP_NATIVE_STEERING"] = "1"
	return startJaz(t, testContext(t), store, root, env)
}

// failingProvider is the agent environment whose prompts can start a run of
// provider failures that outlasts a restart.
func failingProvider(root string) map[string]string {
	return map[string]string{"JAZ_FAKE_ACP_FAILURE_FILE": filepath.Join(root, "failures")}
}

func (p *jazProcess) status(thread string) acp.Job {
	p.t.Helper()
	job, err := p.manager.Status(thread)
	if err != nil {
		p.t.Fatal(err)
	}
	return job
}

func (p *jazProcess) session(thread string) storage.Session {
	p.t.Helper()
	session, err := p.store.LoadSession(thread)
	if err != nil {
		p.t.Fatal(err)
	}
	return session
}

// The 09:00 routine whose provider was at capacity: Jaz goes on with the
// same conversation until the provider answers, and the run ends ok.
func TestARoutineTurnTheProviderFailsContinuesUntilItAnswers(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond, time.Millisecond, time.Millisecond)
	bot := jaz.bot("Events")
	routine := jaz.routine(loops.CreateLoop{Name: "Table", Prompt: "send today's table provider fails 2 overloaded", BotID: bot.ID, Thread: loops.ThreadMain})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}

	jaz.waitRun(run.ID, loops.RunStatusOK)
	// The run closes before its turn does.
	jaz.eventually("the bot's turn ending", func() bool { return jaz.session(bot.ID).Turn == nil })
	prompts := jaz.prompts()
	if len(prompts) != 3 || jaz.promptsContaining("provider fails") != 1 || jaz.promptsContaining(continuation) != 2 {
		t.Fatalf("prompts = %q, want the routine once and two continuations", prompts)
	}
	if session := jaz.session(bot.ID); session.Status != storage.StatusIdle || session.Error != "" {
		t.Fatalf("bot after the answer = %s %q", session.Status, session.Error)
	}
	events, err := jaz.store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	var notices []string
	for _, event := range events {
		if event.ACP != nil && strings.Contains(event.ACP.Error, "Retrying in") {
			notices = append(notices, event.ACP.Error)
		}
	}
	if len(notices) != 2 || !strings.HasPrefix(notices[0], "Selected model is at capacity.") || !strings.HasSuffix(notices[1], "(2 of 3).") {
		t.Fatalf("transcript retry notices = %q", notices)
	}
}

// A capacity failure that follows the provider retrying its connection in the
// same turn is one the provider did not retry, so Jaz does.
func TestACapacityFailureAfterTheProviderReconnectedIsRetried(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond)
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip provider fails 1 reconnected", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}

	jaz.eventually("the turn ending", func() bool { return jaz.session(thread.ID).Turn == nil })
	if session := jaz.session(thread.ID); session.Status != storage.StatusIdle || jaz.promptsContaining(continuation) != 1 {
		t.Fatalf("thread = %s %q, prompts = %q; want one continuation and an answer", session.Status, session.Error, jaz.prompts())
	}
}

// A failure that outlasts every retry ends the turn as failed: the run closes
// as an error with the provider's reason.
func TestARoutineTurnTheProviderKeepsFailingClosesItsRunAsAnError(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond, time.Millisecond, time.Millisecond)
	bot := jaz.bot("Events")
	routine := jaz.routine(loops.CreateLoop{Name: "Table", Prompt: "send today's table provider fails 9 overloaded", BotID: bot.ID, Thread: loops.ThreadMain})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}

	failed := jaz.waitRun(run.ID, loops.RunStatusError)
	if !strings.HasPrefix(failed.Error, "Selected model is at capacity.") || jaz.promptsContaining(continuation) != 3 {
		t.Fatalf("run = %+v, prompts = %q; want three continuations, then the provider's reason", failed, jaz.prompts())
	}
	jaz.eventually("the bot's turn ending", func() bool { return jaz.session(bot.ID).Turn == nil })
	if session := jaz.session(bot.ID); session.Status != storage.StatusError || !strings.HasPrefix(session.Error, "Selected model is at capacity.") {
		t.Fatalf("bot after the failed turn = %s %q", session.Status, session.Error)
	}
}

// Failures the provider says a retry cannot fix, or that it already retried
// itself, reach the bot that started the work as failed at once.
func TestAWorkerTurnEndedByAFailureNotToRetryReportsFailedToItsBot(t *testing.T) {
	for _, kind := range []string{"rejected", "retried", "throttled"} {
		t.Run(kind, func(t *testing.T) {
			jaz := startFailingJaz(t, time.Millisecond, time.Millisecond, time.Millisecond)
			bot := jaz.bot("Gimli")
			jaz.write(bot.ID, "hello")
			worker, err := jaz.manager.Spawn(jaz.ctx, acp.SpawnRequest{ParentID: bot.ID, Slug: "worker", Commitment: owe(t, jaz.store, bot.ID)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: worker.SessionID, Message: "build it provider fails 1 " + kind, Completion: acp.CompletionAsync, ParentVisible: true}); err != nil {
				t.Fatal(err)
			}

			jaz.eventually("the worker's turn ending", func() bool { return jaz.session(worker.SessionID).Turn == nil })
			if got := jaz.promptsContaining(continuation); got != 0 {
				t.Fatalf("a failure not to retry was continued %d times", got)
			}
			if session := jaz.session(worker.SessionID); session.Status != storage.StatusError || session.Error == "" {
				t.Fatalf("worker = %s %q", session.Status, session.Error)
			}
			jaz.eventually("the bot hearing of the failure", func() bool { return jaz.promptsContaining("completed with state failed") == 1 })
		})
	}
}

// Stopping a turn that waits to retry ends it at once and sends nothing more;
// steering it meanwhile is refused, so the server queues the message.
func TestStoppingATurnWaitingToRetryEndsItWithoutAContinuation(t *testing.T) {
	jaz := startFailingJaz(t, time.Hour)
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip provider fails 1 overloaded", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the turn waiting to retry", func() bool { return strings.Contains(jaz.status(thread.ID).Error, "Retrying in") })
	if job := jaz.status(thread.ID); job.State != acp.StateRunning {
		t.Fatalf("a turn waiting to retry = %s", job.State)
	}
	if _, err := jaz.manager.Steer(jaz.ctx, acp.SteerRequest{Session: thread.ID, Message: "also book the hotel"}); !errors.Is(err, acp.ErrSteeringUnsupported) {
		t.Fatalf("steering a turn waiting to retry = %v, want it queued instead", err)
	}

	started := time.Now()
	job, err := jaz.manager.Cancel(jaz.ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateCancelled || time.Since(started) > 2*time.Second {
		t.Fatalf("stop = %s after %s, want cancelled at once", job.State, time.Since(started))
	}
	if prompts := jaz.prompts(); len(prompts) != 1 {
		t.Fatalf("prompts = %q, want only the failed one", prompts)
	}
	if session := jaz.session(thread.ID); session.Status != storage.StatusIdle {
		t.Fatalf("stopped thread = %s %q", session.Status, session.Error)
	}
}

// A native steer answers once the turn it joined ends; when a provider
// failure ended that turn, the turn failed.
func TestATurnANativeSteerJoinedEndsAsTheProviderFailureReports(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond)
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "block until cancelled", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ID)
	if _, err := jaz.manager.Steer(jaz.ctx, acp.SteerRequest{Session: thread.ID, Message: "and the hotel provider fails 1 rejected"}); err != nil {
		t.Fatal(err)
	}

	jaz.eventually("the turn ending", func() bool { return jaz.session(thread.ID).Turn == nil })
	if session := jaz.session(thread.ID); session.Status != storage.StatusError || session.Error != "The requested model is not supported." {
		t.Fatalf("thread after the steered turn failed = %s %q", session.Status, session.Error)
	}
}

// A restart while a turn waits to retry resumes it once, and the retries it
// already used still count.
func TestRestartWhileATurnWaitsToRetryResumesItWithItsRetriesUsed(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond, time.Hour, time.Millisecond)
	bot := jaz.bot("Events")
	routine := jaz.routine(loops.CreateLoop{Name: "Table", Prompt: "send today's table provider fails 9 overloaded", BotID: bot.ID, Thread: loops.ThreadMain})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the second wait", func() bool { return strings.HasSuffix(jaz.status(bot.ID).Error, "(2 of 3).") })

	jaz = jaz.restart(failingProvider(jaz.root))
	failed := jaz.waitRun(run.ID, loops.RunStatusError)
	if !strings.HasPrefix(failed.Error, "Selected model is at capacity.") || jaz.promptsContaining(continuation) != 3 {
		t.Fatalf("run = %+v, prompts = %q; want the resume to use the last retry", failed, jaz.prompts())
	}
}

// A run that closed as an error before its turn's bookkeeping ran finishes
// the turn as failed after a restart.
func TestRestartAfterARunClosedAsAnErrorFinishesItsTurnAsFailed(t *testing.T) {
	store, root := newTestStore(t)
	ctx := testContext(t)
	jaz := startJaz(t, ctx, store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Report", Prompt: "write the report until released", BotID: bot.ID})
	run, err := jaz.loops.RunNow(ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := jaz.waitRun(run.ID, loops.RunStatusRunning).ThreadID
	waitForBlockingTool(t, ctx, jaz.manager, child)
	// Jaz dies after the run closes and before the turn's own bookkeeping.
	jaz.manager.TurnFinished = func(context.Context, acp.Job) {
		_ = jaz.loops.Finish(run.ID, loops.RunStatusError, "Selected model is at capacity.", false)
	}
	jaz.release()
	jaz.waitRun(run.ID, loops.RunStatusError)
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
	jaz.eventually("the child's turn closing as failed", func() bool {
		session, err := store.LoadSession(child)
		return err == nil && session.Turn == nil && session.Status == storage.StatusError && session.Error == "Selected model is at capacity."
	})
}

// A notice the provider sends while it works stays visible in the answer, as
// it was before Jaz asked for failure records.
func TestAProviderNoticeShowsInTheTurnsAnswer(t *testing.T) {
	jaz := startFailingJaz(t, time.Millisecond)
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: jaz.root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "hello provider notice Model metadata not found.", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the answer", func() bool { return jaz.session(thread.ID).Status == storage.StatusIdle })
	events, err := jaz.store.LoadSessionEvents(thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	var answer string
	for _, event := range events {
		answer += event.Content
	}
	if !strings.HasPrefix(answer, "Warning: Model metadata not found.\n\nhello from fake agent") {
		t.Fatalf("answer = %q, want the notice before it", answer)
	}
}

// unrecordedRetries is storage that cannot record a turn's retries.
type unrecordedRetries struct {
	*sqlitestore.Store
}

func (unrecordedRetries) SetTurnRetries(string, int) error {
	return errors.New("disk full")
}

// A retry Jaz cannot record would let a restart retry the turn again, so the
// turn fails instead of waiting to retry.
func TestATurnWhoseRetryCannotBeRecordedFails(t *testing.T) {
	acp.SetProviderRetryDelays(t, time.Millisecond)
	store, root := newTestStore(t)
	env := failingProvider(root)
	env["JAZ_FAKE_ACP_REQUEST_LOG"] = filepath.Join(root, "requests.jsonl")
	manager := newFakeNamedAgentManagerWithOptions(t, unrecordedRetries{store}, root, acp.AgentCodex, env, "", "")
	t.Cleanup(manager.Close)
	ctx := testContext(t)
	thread, err := manager.CreateSession(ctx, acp.SpawnRequest{Slug: "chat", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip provider fails 1 overloaded", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}

	job, err := manager.Wait(ctx, acp.WaitRequest{Session: thread.ID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	jaz := &jazProcess{t: t, root: root}
	if job.State != acp.StateFailed || !strings.HasPrefix(job.Error, "Selected model is at capacity.") || jaz.promptsContaining(continuation) != 0 {
		t.Fatalf("turn = %s %q, prompts = %q; want it failed without a continuation", job.State, job.Error, jaz.prompts())
	}
}

// Once the continuation goes out, the thread no longer shows the failure it
// retries.
func TestAContinuationClearsTheRetryNotice(t *testing.T) {
	acp.SetProviderRetryDelays(t, time.Millisecond)
	store, root := newTestStore(t)
	env := failingProvider(root)
	env["JAZ_FAKE_ACP_HOLD_TEXT"] = continuation
	jaz := startJaz(t, testContext(t), store, root, env)
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "chat", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.ID, Message: "plan the trip provider fails 1 overloaded", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ID)

	events, err := store.LoadSessionEvents(thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	for _, event := range events {
		if event.Type == "acp" && event.ACP != nil {
			status = event.ACP.Error
		}
	}
	if status != "" {
		t.Fatalf("the thread's status while the continuation works = %q, want no failure", status)
	}
	jaz.release()
}
