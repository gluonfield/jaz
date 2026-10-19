package acp_test

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

// fakeBackgroundWork makes a prompt start background work that outlives its
// turn, as a background command does in Claude Code, and reports the agent's
// work as the Claude adapter does. The test lets it go on step by step by
// writing <release file>-<name> (the work finishes), then -<name>-run (the
// agent's own run about the work that finished gives its first output), then
// -<name>-done (the run ends); with "at once" every step follows the turn at
// once.
//
// Work that finishes while a prompt's turn runs is taken into that turn,
// except work named late, which finishes during the turn's last model call.
// Other finished work waits for the agent's own run, which takes in all the
// work waiting when it begins. A prompt that comes after work finished and
// before the run ended is taken into the run, as Claude Code takes it in, and
// the run ends with that prompt's turn. A prompt that says "after the run"
// instead waits for the run to end and runs as a turn of its own. The run
// about work named broken fails. With "at once", the work finishes as the
// turn ends and the run follows its response at once; the fake then writes
// <release file>-<name>-emitted. With "untold", Claude Code never tells the
// model the work finished. With "quietly", the agent reports neither its work
// nor a run of its own, as agents other than Claude do.
//
// "start background agent <name>" starts a background subagent instead: the
// adapter reports the turn's work ended at its result, but holds the turn's
// response until the subagent's follow-up, a cycle of its own, has reported
// (after <release file>-<name>).
//
// Each idle report names the work its turn took in, as the Claude adapter
// does, unless JAZ_FAKE_ACP_UNNAMED=1. With JAZ_FAKE_ACP_WORK_STATE=1, every
// other prompt's turn reports its work as well.
var fakeBackgroundWork = regexp.MustCompile(`start background work (\w+)( at once| quietly| untold)?`)

var fakeStepHold = regexp.MustCompile(`work until step (\w+)`)

var fakeRunMessages atomic.Int64

// fakeRun is the agent's own run about finished work; the fake runs one
// session. waiting: finished work no model call took in yet. taken: the work
// the run or the prompt's turn took in. running: the run goes on. inTurn: a
// prompt's turn runs.
var fakeRun struct {
	mu      sync.Mutex
	waiting []string
	taken   []string
	running bool
	inTurn  bool
}

func startFakeBackgroundWork(conn jsonrpc.MessageConn, msg *jsonrpc.Message) bool {
	match := fakeBackgroundWork.FindStringSubmatch(string(msg.Params))
	if match == nil {
		return false
	}
	name, atOnce, quiet := match[1], match[2] == " at once", match[2] == " quietly"
	if !quiet {
		fakeWorkState(conn, map[string]any{"state": "running"}, nil)
	}
	notify(conn, "session/update", map[string]any{
		"sessionId": fakeSessionID,
		"update":    map[string]any{"sessionUpdate": "async_task_spawned", "asyncTaskId": name, "name": name, "taskType": "shell", "canStop": true},
	})
	fakeText(conn, "Started "+name+" in the background.")
	if !quiet {
		fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, nil)
	}
	finish := func() {
		notify(conn, "session/update", map[string]any{
			"sessionId": fakeSessionID,
			"update":    map[string]any{"sessionUpdate": "async_task_state_update", "asyncTaskId": name, "state": "completed"},
		})
		switch {
		case quiet || match[2] == " untold":
		case fakeRun.inTurn && name != "late":
			fakeRun.taken = append(fakeRun.taken, name)
		default:
			fakeRun.waiting = append(fakeRun.waiting, name)
		}
	}
	sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
	if atOnce {
		fakeRun.mu.Lock()
		defer fakeRun.mu.Unlock()
		finish()
		fakeRunBegins(conn)
		fakeRunEnds(conn)
		if err := os.WriteFile(os.Getenv("JAZ_FAKE_ACP_RELEASE_FILE")+"-"+name+"-emitted", nil, 0o600); err != nil {
			panic(err)
		}
		return true
	}
	go func() {
		step := func(suffix string) {
			waitForFakeFile(os.Getenv("JAZ_FAKE_ACP_RELEASE_FILE") + "-" + name + suffix)
		}
		step("")
		fakeRun.mu.Lock()
		finish()
		fakeRun.mu.Unlock()
		step("-run")
		fakeRun.mu.Lock()
		if len(fakeRun.waiting) > 0 && !fakeRun.running && !fakeRun.inTurn {
			fakeRunBegins(conn)
		}
		fakeRun.mu.Unlock()
		step("-done")
		fakeRun.mu.Lock()
		defer fakeRun.mu.Unlock()
		if fakeRun.running {
			fakeRunEnds(conn)
		}
	}()
	return true
}

var fakeBackgroundAgent = regexp.MustCompile(`start background agent (\w+)`)

func startFakeBackgroundAgent(conn jsonrpc.MessageConn, msg *jsonrpc.Message) bool {
	match := fakeBackgroundAgent.FindStringSubmatch(string(msg.Params))
	if match == nil {
		return false
	}
	name := match[1]
	fakeWorkState(conn, map[string]any{"state": "running"}, nil)
	fakeText(conn, "Launched "+name+" in the background.")
	fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, nil)
	go func() {
		waitForFakeFile(os.Getenv("JAZ_FAKE_ACP_RELEASE_FILE") + "-" + name)
		fakeRun.mu.Lock()
		defer fakeRun.mu.Unlock()
		fakeWorkState(conn, map[string]any{"state": "running"}, nil)
		fakeText(conn, name+" reported: all green.")
		fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, []string{name})
		sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
	}()
	return true
}

// answerDuringFakeRun answers a prompt that comes while finished work waits
// for the agent's own run or the run goes on.
func answerDuringFakeRun(conn jsonrpc.MessageConn, msg *jsonrpc.Message) bool {
	fakeRun.mu.Lock()
	defer fakeRun.mu.Unlock()
	if len(fakeRun.waiting) == 0 && !fakeRun.running {
		return false
	}
	if !fakeRun.running {
		fakeRunBegins(conn)
	}
	if strings.Contains(string(msg.Params), "after the run") {
		fakeRunEnds(conn)
		fakeWorkState(conn, map[string]any{"state": "running"}, nil)
		fakeText(conn, "Answered after the run.")
		fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, nil)
	} else {
		fakeText(conn, "Took the message into the run.")
		taken := fakeRun.taken
		fakeRun.running, fakeRun.taken = false, nil
		fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, taken)
	}
	sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
	return true
}

// fakeTurnRuns opens a prompt's turn.
func fakeTurnRuns(conn jsonrpc.MessageConn) {
	if os.Getenv("JAZ_FAKE_ACP_WORK_STATE") != "1" {
		return
	}
	fakeRun.mu.Lock()
	defer fakeRun.mu.Unlock()
	fakeRun.inTurn = true
	fakeWorkState(conn, map[string]any{"state": "running"}, nil)
}

// fakeTurnEnds ends a prompt's turn, reporting the work it took in.
func fakeTurnEnds(conn jsonrpc.MessageConn) {
	if os.Getenv("JAZ_FAKE_ACP_WORK_STATE") != "1" {
		return
	}
	fakeRun.mu.Lock()
	defer fakeRun.mu.Unlock()
	taken := fakeRun.taken
	for _, name := range taken {
		fakeText(conn, name+" finished: all green.")
	}
	fakeRun.inTurn, fakeRun.taken = false, nil
	fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn"}, taken)
}

// fakeTurnFails ends a prompt's turn with the provider failure in meta, as
// the Claude adapter reports it before the prompt's response.
func fakeTurnFails(conn jsonrpc.MessageConn, meta map[string]any) {
	if os.Getenv("JAZ_FAKE_ACP_WORK_STATE") != "1" {
		return
	}
	fakeRun.mu.Lock()
	defer fakeRun.mu.Unlock()
	fakeRun.inTurn = false
	fakeWorkState(conn, map[string]any{"state": "idle", "stopReason": "end_turn", "_meta": meta}, nil)
}

// fakeRunBegins begins the agent's own run, which takes in the work waiting;
// Claude Code names that work only at the run's end (see fakeRunEnds).
func fakeRunBegins(conn jsonrpc.MessageConn) {
	fakeRun.running = true
	fakeRun.taken, fakeRun.waiting = fakeRun.waiting, nil
	fakeWorkState(conn, map[string]any{"state": "running"}, nil)
	for _, name := range fakeRun.taken {
		fakeText(conn, name+" finished: all green.")
	}
}

func fakeRunEnds(conn jsonrpc.MessageConn) {
	taken := fakeRun.taken
	fakeRun.running, fakeRun.taken = false, nil
	if slices.Contains(taken, "broken") {
		fakeWorkState(conn, map[string]any{
			"state": "idle", "stopReason": "error",
			"error": map[string]any{"code": -32603, "message": "API Error: Overloaded"},
		}, taken)
		return
	}
	fakeWorkState(conn, map[string]any{
		"state": "idle", "stopReason": "end_turn",
		"usage": map[string]any{"inputTokens": 7, "outputTokens": 4, "totalTokens": 11},
	}, taken)
}

// fakeWorkState reports the agent's work, naming the finished work taken in.
func fakeWorkState(conn jsonrpc.MessageConn, state map[string]any, taken []string) {
	if os.Getenv("JAZ_FAKE_ACP_UNNAMED") != "1" {
		state["tasks"] = append([]string{}, taken...)
	}
	notify(conn, "session/update", map[string]any{
		"sessionId": fakeSessionID,
		"update":    map[string]any{"sessionUpdate": "session_info_update", "_meta": map[string]any{"jaz": map[string]any{"stateUpdate": state}}},
	})
}

func fakeText(conn jsonrpc.MessageConn, text string) {
	notify(conn, "session/update", map[string]any{
		"sessionId": fakeSessionID,
		"update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"messageId":     "run-message-" + strconv.FormatInt(fakeRunMessages.Add(1), 10),
			"content":       map[string]any{"type": "text", "text": text},
		},
	})
}

// waitForFakeFile waits for path, and exits once Jaz is gone.
func waitForFakeFile(path string) {
	for _, err := os.Stat(path); err != nil; _, err = os.Stat(path) {
		if os.Getppid() == 1 {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
