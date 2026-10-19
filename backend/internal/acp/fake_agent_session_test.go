package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

// fakeTaskReport makes a prompt report a background task in a state.
var fakeTaskReport = regexp.MustCompile(`task (\w+)=(\w+)`)

func reportFakeTasks(conn jsonrpc.MessageConn, params json.RawMessage) {
	for _, report := range fakeTaskReport.FindAllStringSubmatch(string(params), -1) {
		kind := "async_task_state_update"
		if report[2] == "running" {
			kind = "async_task_spawned"
		}
		notify(conn, "session/update", map[string]any{
			"sessionId": fakeSessionID,
			"update":    map[string]any{"sessionUpdate": kind, "asyncTaskId": report[1], "name": report[1], "state": report[2]},
		})
	}
}

// fakeSessionClaim, with JAZ_FAKE_ACP_SESSION_CLAIM set, holds a claim file
// from opening the session until the process has exited, as a provider holds
// its session's files, and takes a moment to shut down. No other process can
// open the session meanwhile.
type fakeSessionClaim struct {
	path    string
	claimed atomic.Bool
}

func newFakeSessionClaim() *fakeSessionClaim {
	c := &fakeSessionClaim{path: os.Getenv("JAZ_FAKE_ACP_SESSION_CLAIM")}
	if c.path != "" {
		terminated := make(chan os.Signal, 1)
		signal.Notify(terminated, syscall.SIGTERM)
		go func() {
			<-terminated
			c.exit()
		}()
	}
	return c
}

// take claims the session for a request that opens it, refusing the request
// while another process holds the claim.
func (c *fakeSessionClaim) take(conn jsonrpc.MessageConn, msg *jsonrpc.Message) bool {
	opens := msg.Method == "session/new" || msg.Method == "session/load" || msg.Method == "session/resume"
	if c.path == "" || !opens || c.claimed.Load() {
		return true
	}
	file, err := os.OpenFile(c.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InternalError("session is open in another process", nil))
		_ = conn.Send(context.Background(), resp)
		return false
	}
	_ = file.Close()
	c.claimed.Store(true)
	return true
}

func (c *fakeSessionClaim) exit() {
	if c.claimed.Load() {
		time.Sleep(300 * time.Millisecond)
		_ = os.Remove(c.path)
	}
	os.Exit(0)
}

// fakeAskAfterTurn ends the turn, then asks the person a question outside it.
func fakeAskAfterTurn(conn jsonrpc.MessageConn, prompt *jsonrpc.Message) {
	notify(conn, "session/update", map[string]any{
		"sessionId": fakeSessionID,
		"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "fake:asking", "content": map[string]any{"type": "text", "text": "one question next"}},
	})
	sendResult(conn, prompt, map[string]any{"stopReason": "end_turn"})
	// The turn's bookkeeping settles before the question arrives.
	time.Sleep(50 * time.Millisecond)
	req, _ := jsonrpc.NewRequest(json.RawMessage(`"fake-elicit-after"`), "elicitation/create", map[string]any{
		"mode":      "form",
		"sessionId": fakeSessionID,
		"message":   "Which workspace?",
		"requestedSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"choice": map[string]any{"type": "string", "title": "Choice"}},
		},
	})
	_ = conn.Send(context.Background(), req)
}

// fakeFinalReply answers as Codex and Claude do: a line on what it does
// next, a tool call, a note, then its final reply in chunks of one message,
// and any provider notice the prompt names as the turn ends.
func fakeFinalReply(conn jsonrpc.MessageConn, prompt *jsonrpc.Message, nextMessageID func(string) string, typed bool) {
	write := func(id, text, phase string) {
		update := map[string]any{
			"sessionUpdate": "agent_message_chunk", "messageId": id,
			"content": map[string]any{"type": "text", "text": text},
		}
		if phase != "" {
			update["_meta"] = map[string]any{"jetbrains": map[string]any{"air": map[string]any{"version": 1, "phase": phase}}}
		}
		notify(conn, "session/update", map[string]any{
			"sessionId": fakeSessionID,
			"update":    update,
		})
	}
	phase, commentary := "final_answer", "commentary"
	if strings.Contains(string(prompt.Params), "unclassified reply") {
		phase, commentary = "", ""
	}
	write(nextMessageID("progress"), "Let me check the log.", commentary)
	notify(conn, "session/update", map[string]any{
		"sessionId": fakeSessionID,
		"update":    map[string]any{"sessionUpdate": "tool_call", "toolCallId": nextMessageID("tool"), "title": "git log", "status": "completed"},
	})
	write(nextMessageID("note"), "Found it.", commentary)
	if strings.Contains(string(prompt.Params), "commentary only") {
		sendResult(conn, prompt, map[string]any{"stopReason": "end_turn"})
		return
	}
	final := nextMessageID("final")
	write(final, "The last commit ", phase)
	write(final, "added due dates.", phase)
	fakeProviderNoticeUpdate(conn, prompt, typed)
	sendResult(conn, prompt, map[string]any{"stopReason": "end_turn"})
}

// openFakeSession, with JAZ_FAKE_ACP_SESSION_PER_CWD set, names the session
// after its directory, so threads in their own directories keep their own
// sessions across processes. With JAZ_FAKE_ACP_RESTORE_TOGETHER=dir:n, a
// restore waits until n restores are under way and fails after five seconds
// alone.
func openFakeSession(cwd string, restoring bool) bool {
	if os.Getenv("JAZ_FAKE_ACP_SESSION_PER_CWD") == "1" {
		fakeSessionID = "fake-session-" + filepath.Base(cwd)
	}
	dir, count, together := strings.Cut(os.Getenv("JAZ_FAKE_ACP_RESTORE_TOGETHER"), ":")
	if !restoring || !together {
		return true
	}
	want, _ := strconv.Atoi(count)
	marker := filepath.Join(dir, strconv.Itoa(os.Getpid()))
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(marker, nil, 0o600)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if entries, _ := os.ReadDir(dir); len(entries) >= want {
			return true
		}
	}
	_ = os.Remove(marker)
	return false
}
