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
	manager := newFakeAgentManagerWithOptions(t, newProviderSubagentStore(t), t.TempDir(), env, "fake-large", "high")
	ctx, id := startIdleThread(t, manager, "fake")
	stopIdleProcesses(t, manager, 500*time.Millisecond)
	time.Sleep(250 * time.Millisecond)
	if !processRunning(startedPIDs(t, startLog)[0]) {
		t.Fatal("process stopped before it had been idle for the timeout")
	}
	waitProcessGone(ctx, t, startedPIDs(t, startLog)[0])
	if slices.Contains(manager.LiveSessionRefs(), id) {
		t.Fatal("stopped thread is still live")
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
			manager := newFakeNamedAgentManagerWithOptions(t, newProviderSubagentStore(t), t.TempDir(), tc.agent, env, "", "")
			ctx, id := startIdleThread(t, manager, tc.agent)
			pid := startedPIDs(t, startLog)[0]
			prompts := requestsSent(t, requestLog, "session/prompt")
			started := tc.start(ctx, manager, id)
			for requestsSent(t, requestLog, "session/prompt") == prompts {
				pause(ctx, t)
			}

			stopIdleProcesses(t, manager, 20*time.Millisecond)
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
	manager := newFakeAgentManagerWithOptions(t, newProviderSubagentStore(t), t.TempDir(), env, "", "")
	ctx, id := startIdleThread(t, manager, "fake")
	stopIdleProcesses(t, manager, time.Millisecond)
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
	manager := newFakeAgentManagerWithOptions(t, newProviderSubagentStore(t), t.TempDir(), env, "", "")
	ctx, id := startIdleThread(t, manager, "fake")
	stopping := startedPIDs(t, startLog)[0]
	stopIdleProcesses(t, manager, 20*time.Millisecond)
	for slices.Contains(manager.LiveSessionRefs(), id) {
		pause(ctx, t)
	}
	if !processRunning(stopping) {
		t.Fatal("the stopped process exited before the follow-up; the test cannot race it")
	}
	if err := turn(ctx, manager, id, "while the old process exits"); err != nil {
		t.Fatal(err)
	}
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
func stopIdleProcesses(t *testing.T, manager *acp.Manager, idleFor time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.StopIdleProcesses(ctx, idleFor)
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

// settled waits for the thread's turn to end, reading the thread from storage
// once its process has been stopped.
func settled(ctx context.Context, manager *acp.Manager, id, message string) error {
	for {
		job, err := manager.Status(id)
		if err != nil {
			return err
		}
		switch job.State {
		case acp.StateRunning, acp.StateStarting:
		case acp.StateIdle, acp.StateNotRunning:
			return nil
		default:
			return fmt.Errorf("%q ended %s: %s", message, job.State, job.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
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
