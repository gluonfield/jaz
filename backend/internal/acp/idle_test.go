//go:build !windows

package acp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
)

func TestIdleProcessStopsAndItsThreadResumesTheNativeSession(t *testing.T) {
	logs := t.TempDir()
	startLog, requestLog := filepath.Join(logs, "starts"), filepath.Join(logs, "requests")
	// The fake refuses a prompt unless this process was given the thread's
	// model and effort, so a resumed process must restore both.
	env := map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog, "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
		"JAZ_FAKE_ACP_SET_MODEL": "1", "JAZ_FAKE_ACP_EXPECT_MODEL": "fake-large",
		"JAZ_FAKE_ACP_SET_CONFIG": "1", "JAZ_FAKE_ACP_EXPECT_EFFORT": "high",
	}
	manager := idleManager(t, "fake", env, "fake-large", "high")
	ctx, id := startIdleThread(t, manager, "fake")
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 500 * time.Millisecond})
	time.Sleep(250 * time.Millisecond)
	if !processRunning(startedPIDs(t, startLog)[0]) {
		t.Fatal("process stopped before it had been idle for the timeout")
	}
	waitProcessGone(ctx, t, startedPIDs(t, startLog)[0])
	if !manager.SessionUsed(id) {
		t.Fatal("a stopped thread no longer counts as in use, so its worktree could be pruned")
	}

	if err := turn(ctx, manager, id, "after the stop"); err != nil {
		t.Fatal(err)
	}
	if starts := processStarts(t, startLog); starts != 2 {
		t.Fatalf("process starts = %d, want a second process for the follow-up", starts)
	}
	if created, resumed := requestsSent(t, requestLog, "session/new"), requestsSent(t, requestLog, "session/resume"); created != 1 || resumed != 1 {
		t.Fatalf("session/new = %d, session/resume = %d; want the native session resumed, not replaced", created, resumed)
	}
}

// A Claude bot renders its changing prompt on every launch, so it keeps its
// process for the bot timeout; other bots keep native caching and stop with
// ordinary threads.
func TestIdleClaudeBotKeepsItsProcessForTheBotTimeout(t *testing.T) {
	for _, tc := range []struct {
		agent string
		kept  bool
	}{{acp.AgentClaude, true}, {acp.AgentCodex, false}} {
		t.Run(tc.agent, func(t *testing.T) {
			startLog := filepath.Join(t.TempDir(), "starts")
			env := map[string]string{"JAZ_FAKE_ACP_START_LOG": startLog, "JAZ_FAKE_ACP_UNIQUE_SESSION": "1"}
			manager := idleManager(t, tc.agent, env, "", "")
			ctx, _ := startIdleThread(t, manager, tc.agent)
			bot, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: tc.agent, Slug: "idle-bot", MCPServerPolicy: acp.MCPServerPolicyBot})
			if err != nil {
				t.Fatal(err)
			}
			if err := turn(ctx, manager, bot.SessionID, "first"); err != nil {
				t.Fatal(err)
			}
			pids := startedPIDs(t, startLog)
			timeouts := acp.IdleTimeouts{Thread: 20 * time.Millisecond, Bot: 3 * time.Second}
			started := time.Now()
			stopIdleProcesses(t, manager, timeouts)
			waitProcessGone(ctx, t, pids[0])
			if !tc.kept {
				waitProcessGone(ctx, t, pids[1])
				if time.Since(started) >= timeouts.Bot {
					t.Fatal("kept a bot that keeps native caching for the bot timeout")
				}
				return
			}
			time.Sleep(300 * time.Millisecond)
			if !processRunning(pids[1]) {
				t.Fatal("stopped a Claude bot's process at the thread timeout")
			}
			waitProcessGone(ctx, t, pids[1])
		})
	}
}

func TestIdleStopWaitsForWorkInTheProcess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		agent  string
		start  func(context.Context, *acp.Manager, string) <-chan error
		finish func(context.Context, *acp.Manager, string) error
	}{
		{
			name:  "turn",
			agent: acp.AgentClaude,
			start: func(ctx context.Context, manager *acp.Manager, id string) <-chan error {
				_, err := manager.Send(ctx, acp.SendRequest{Session: id, Message: "until released", Completion: acp.CompletionInline})
				return finished(err)
			},
			finish: func(ctx context.Context, manager *acp.Manager, id string) error {
				return settled(ctx, manager, id, "until released")
			},
		},
		{
			name:  "background task",
			agent: acp.AgentClaude,
			start: func(ctx context.Context, manager *acp.Manager, id string) <-chan error {
				return finished(turn(ctx, manager, id, "task build=running"))
			},
			finish: func(ctx context.Context, manager *acp.Manager, id string) error {
				return turn(ctx, manager, id, "task build=completed")
			},
		},
		{
			name:  "provider subagent",
			agent: acp.AgentClaude,
			start: func(ctx context.Context, manager *acp.Manager, id string) <-chan error {
				return finished(turn(ctx, manager, id, "subagent toolu_bg=running"))
			},
			finish: func(ctx context.Context, manager *acp.Manager, id string) error {
				return turn(ctx, manager, id, "subagent toolu_bg=completed")
			},
		},
		{
			name:  "question",
			agent: acp.AgentClaude,
			start: func(ctx context.Context, manager *acp.Manager, id string) <-chan error {
				err := turn(ctx, manager, id, "ask after the turn")
				for err == nil && !manager.AwaitsAnswer(id) {
					select {
					case <-ctx.Done():
						err = ctx.Err()
					case <-time.After(time.Millisecond):
					}
				}
				return finished(err)
			},
			finish: func(ctx context.Context, manager *acp.Manager, id string) error {
				job, err := manager.Status(id)
				if err != nil || len(job.Permissions) == 0 {
					return fmt.Errorf("pending question = %v, %v", job.Permissions, err)
				}
				return manager.AnswerInteractive(ctx, acp.InteractiveAnswer{
					Session: id, RequestID: job.Permissions[0].ID,
					Answers: map[string]acp.InteractiveAnswerValue{"choice": {Answers: []string{"main"}}},
				})
			},
		},
		{
			name:  "side chat",
			agent: acp.AgentCodex,
			start: func(ctx context.Context, manager *acp.Manager, id string) <-chan error {
				done := make(chan error, 1)
				go func() {
					done <- manager.SendSideChat(ctx, acp.SideChatRequest{Session: id, ID: "side-1", Message: "until released"})
				}()
				return done
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := t.TempDir()
			startLog, requestLog, release := filepath.Join(logs, "starts"), filepath.Join(logs, "requests"), filepath.Join(logs, "release")
			env := map[string]string{
				"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog, "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
				"JAZ_FAKE_ACP_RELEASE_FILE": release,
			}
			manager := idleManager(t, tc.agent, env, "", "")
			ctx, id := startIdleThread(t, manager, tc.agent)
			pid := startedPIDs(t, startLog)[0]
			prompts := requestsSent(t, requestLog, "session/prompt")
			started := tc.start(ctx, manager, id)
			for requestsSent(t, requestLog, "session/prompt") == prompts {
				pause(ctx, t)
			}

			stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
			time.Sleep(300 * time.Millisecond)
			if !processRunning(pid) {
				t.Fatalf("process stopped while its %s ran", tc.name)
			}

			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := <-started; err != nil {
				t.Fatal(err)
			}
			if tc.finish != nil {
				if err := tc.finish(ctx, manager, id); err != nil {
					t.Fatal(err)
				}
			}
			waitProcessGone(ctx, t, pid)
		})
	}
}

func TestTurnsRacingIdleStopsAllComplete(t *testing.T) {
	startLog := filepath.Join(t.TempDir(), "starts")
	env := map[string]string{"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog}
	manager := idleManager(t, "fake", env, "", "")
	ctx, id := startIdleThread(t, manager, "fake")
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: time.Millisecond})
	// Varied pauses land follow-ups before, during and after stops.
	for i := range 30 {
		time.Sleep(time.Duration(i%5) * 3 * time.Millisecond)
		if err := turn(ctx, manager, id, "turn "+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if starts := processStarts(t, startLog); starts < 3 {
		t.Fatalf("process starts = %d, want turns that resumed stopped processes", starts)
	}
}

func TestResumeWaitsForTheStoppedProcessToExit(t *testing.T) {
	logs := t.TempDir()
	startLog := filepath.Join(logs, "starts")
	env := map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog,
		"JAZ_FAKE_ACP_SESSION_CLAIM": filepath.Join(logs, "claim"),
	}
	manager := idleManager(t, "fake", env, "", "")
	ctx, id := startIdleThread(t, manager, "fake")
	stopping := startedPIDs(t, startLog)[0]
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
	for slices.ContainsFunc(manager.List(), func(job acp.Job) bool { return job.ID == id }) {
		pause(ctx, t)
	}
	if !processRunning(stopping) {
		t.Fatal("the stopped process exited before the follow-up; the test cannot race it")
	}
	if err := turn(ctx, manager, id, "while the old process exits"); err != nil {
		t.Fatal(err)
	}
}

func TestStoppedThreadsResumeInParallel(t *testing.T) {
	logs := t.TempDir()
	startLog := filepath.Join(logs, "starts")
	// Each restore waits until four are under way, failing if it waits alone.
	env := map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog, "JAZ_FAKE_ACP_SESSION_PER_CWD": "1",
		"JAZ_FAKE_ACP_RESTORE_TOGETHER": filepath.Join(logs, "restoring") + ":4",
	}
	manager := idleManager(t, "fake", env, "", "")
	ctx, first := startIdleThread(t, manager, "fake")
	ids := []string{first}
	for i := range 3 {
		spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "idle-parallel-" + strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		if err := turn(ctx, manager, spawned.SessionID, "first"); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, spawned.SessionID)
	}
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
	for _, pid := range startedPIDs(t, startLog) {
		waitProcessGone(ctx, t, pid)
	}

	errs := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			errs <- turn(ctx, manager, id, "after the stop")
		}()
	}
	for range ids {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// idleManager starts a manager whose turns end with Jaz's turn bookkeeping,
// so a thread whose process stopped reads from storage as its turn left it.
func idleManager(t *testing.T, agent string, env map[string]string, model, effort string) *acp.Manager {
	t.Helper()
	store := newProviderSubagentStore(t)
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), agent, env, model, effort)
	manager.Events = sessionevents.New()
	manager.TurnFinished = (&server.Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: manager.Events}).HandleACPTurnFinished
	return manager
}

// startIdleThread spawns a thread on agent and finishes its first turn.
func startIdleThread(t *testing.T, manager *acp.Manager, agent string) (context.Context, string) {
	t.Helper()
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: agent, Slug: "idle-" + agent})
	if err != nil {
		t.Fatal(err)
	}
	if err := turn(ctx, manager, spawned.SessionID, "first"); err != nil {
		t.Fatal(err)
	}
	return ctx, spawned.SessionID
}

// stopIdleProcesses runs the idle stopper for the rest of the test.
func stopIdleProcesses(t *testing.T, manager *acp.Manager, timeouts acp.IdleTimeouts) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.StopIdleProcesses(ctx, timeouts)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// turn sends message and waits for its turn to end.
func turn(ctx context.Context, manager *acp.Manager, id, message string) error {
	if _, err := manager.Send(ctx, acp.SendRequest{Session: id, Message: message, Completion: acp.CompletionInline}); err != nil {
		return err
	}
	return settled(ctx, manager, id, message)
}

// settled waits for the thread's turn to end as wait_threads does: until Jaz
// has recorded the end, which is what the thread reads once its process stops.
func settled(ctx context.Context, manager *acp.Manager, id, message string) error {
	result, err := manager.WaitThreads(ctx, []string{id}, time.Minute)
	if err != nil {
		return err
	}
	if len(result.Threads) == 0 {
		return fmt.Errorf("%q: %s", message, result.Errors[id])
	}
	switch job := result.Threads[0]; job.State {
	case acp.StateIdle, acp.StateNotRunning:
		return nil
	default:
		return fmt.Errorf("%q ended %s: %s", message, job.State, job.Error)
	}
}

func finished(err error) <-chan error {
	done := make(chan error, 1)
	done <- err
	return done
}

func pause(ctx context.Context, t *testing.T) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(time.Millisecond):
	}
}

// requestsSent counts the requests for method in a fake agent's request log.
func requestsSent(t *testing.T, path, method string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), `"method":"`+method+`"`)
}

func processRunning(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitProcessGone(ctx context.Context, t *testing.T, pid int) {
	t.Helper()
	for processRunning(pid) {
		pause(ctx, t)
	}
}

func TestDetachedQuestionDoesNotPinIdleProcess(t *testing.T) {
	starts := filepath.Join(t.TempDir(), "starts")
	manager := idleManager(t, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_START_LOG": starts, "JAZ_FAKE_ACP_RESUME": "1"}, "", "")
	ctx, id := startIdleThread(t, manager, acp.AgentCodex)
	pid := startedPIDs(t, starts)[0]
	events := manager.Events.Subscribe(ctx, id)
	call, cancel := context.WithCancel(ctx)
	done := make(chan acp.AskUserOutput, 1)
	go func() {
		out, _ := manager.AskUser(call, id, acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "q", Question: "Choose one"}}})
		done <- out
	}()
	select {
	case <-events:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
	time.Sleep(100 * time.Millisecond)
	if !processRunning(pid) {
		t.Fatal("live question waiter lost its process")
	}
	cancel()
	<-done
	waitProcessGone(ctx, t, pid)
	if !manager.AwaitsAnswer(id) {
		t.Fatal("idle retirement cancelled the durable question")
	}
	removed := false
	if err := manager.WithStoppedSessions([]string{id}, func() error { removed = true; return nil }); err != nil || removed {
		t.Fatalf("question worktree was released: removed=%v, %v", removed, err)
	}
	if _, err := manager.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	if manager.AwaitsAnswer(id) {
		t.Fatal("explicit stop kept the detached question open")
	}
}

// Codex's plan can stop offering a model for a while, as while a new model
// rolls out. A thread on it runs Codex's default meanwhile and returns to its
// model once its plan offers it again, though the launch is judged against
// the list the last process saw, which still lacks it.
func TestACodexThreadReturnsToAModelItsPlanStoppedOfferingForAWhile(t *testing.T) {
	logs := t.TempDir()
	starts, models, turns := filepath.Join(logs, "starts"), filepath.Join(logs, "models"), filepath.Join(logs, "turns")
	offer := func(list string) {
		if err := os.WriteFile(models, []byte(list), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	offer("gpt-a,gpt-b")
	env := map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": starts, "JAZ_FAKE_ACP_TURN_LOG": turns,
		"JAZ_FAKE_ACP_MODELS_FILE": models, "JAZ_FAKE_ACP_SET_CONFIG": "1",
	}
	manager := idleManager(t, acp.AgentCodex, env, "gpt-b", "")
	ctx, id := startIdleThread(t, manager, acp.AgentCodex)
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
	for i, list := range []string{"gpt-a", "gpt-a,gpt-b"} {
		waitProcessGone(ctx, t, startedPIDs(t, starts)[i])
		offer(list)
		if err := turn(ctx, manager, id, "offered "+list); err != nil {
			t.Fatal(err)
		}
	}
	var ran []string
	for _, turn := range readFakeTurns(t, turns) {
		ran = append(ran, turn.Model)
	}
	if !slices.Equal(ran, []string{"gpt-b", "gpt-a", "gpt-b"}) {
		t.Fatalf("turns ran on %v, want gpt-b, Codex's default while gpt-b was not offered, then gpt-b", ran)
	}
}

func TestACodexThreadKeepsAModelIDCarryingItsEffortAfterAnIdleStop(t *testing.T) {
	logs := t.TempDir()
	starts, turns := filepath.Join(logs, "starts"), filepath.Join(logs, "turns")
	env := map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": starts, "JAZ_FAKE_ACP_TURN_LOG": turns,
		"JAZ_FAKE_ACP_MODELS": "gpt-6-sol/medium,gpt-6-luna/high", "JAZ_FAKE_ACP_SET_CONFIG": "1",
	}
	manager := idleManager(t, acp.AgentCodex, env, "gpt-6-luna/high", "")
	ctx, id := startIdleThread(t, manager, acp.AgentCodex)
	stopIdleProcesses(t, manager, acp.IdleTimeouts{Thread: 20 * time.Millisecond})
	waitProcessGone(ctx, t, startedPIDs(t, starts)[0])
	if err := turn(ctx, manager, id, "again"); err != nil {
		t.Fatal(err)
	}
	want := fakeTurn{Model: "gpt-6-luna", Effort: "high"}
	if turns := readFakeTurns(t, turns); !slices.Equal(turns, []fakeTurn{want, want}) {
		t.Fatalf("turns = %v, want both on %v", turns, want)
	}
}
