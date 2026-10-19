package acp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

// fakeProviderFailures makes a prompt start a run of provider failures: this
// prompt and the next n-1 the session gets end with kind's failure. The count
// lives in JAZ_FAKE_ACP_FAILURE_FILE so it outlasts a restart.
var fakeProviderFailures = regexp.MustCompile(`provider fails (\d+) (overloaded|rejected|retried|reconnected)`)

// fakeProviderNotice makes a prompt report a notice before it answers.
var fakeProviderNotice = regexp.MustCompile(`provider notice ([A-Za-z .]+)`)

// fakeFailureKinds are the records codex-acp and claude-agent-acp send for a
// failure the provider does not retry (overloaded), one the request itself
// causes (rejected), one the provider already retried in the turn (retried),
// and a capacity failure after the provider retried its connection
// (reconnected), which codex-acp reports under the same incident id.
var fakeFailureKinds = map[string]map[string]any{
	"overloaded":  {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}},
	"rejected":    {"category": "request", "title": "The requested model is not supported.", "actions": []string{}},
	"retried":     {"category": "connection", "title": "stream disconnected before completion", "actions": []string{"retry", "new_session"}, "retrying": "connection"},
	"reconnected": {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}, "retrying": "connection"},
}

// fakeTypedFailures reports whether initialize negotiated AIR sessionFailure,
// which makes the adapters return failures as records instead of text.
func fakeTypedFailures(params json.RawMessage) bool {
	var req struct {
		ClientCapabilities struct {
			Meta struct {
				Jetbrains struct {
					Air struct {
						Capabilities []string `json:"capabilities"`
					} `json:"air"`
				} `json:"jetbrains"`
			} `json:"_meta"`
		} `json:"clientCapabilities"`
	}
	_ = json.Unmarshal(params, &req)
	for _, capability := range req.ClientCapabilities.Meta.Jetbrains.Air.Capabilities {
		if capability == "sessionFailure" {
			return true
		}
	}
	return false
}

// fakeProviderNoticeUpdate reports a prompt's notice the way the adapters do.
func fakeProviderNoticeUpdate(conn jsonrpc.MessageConn, msg *jsonrpc.Message, typed bool) {
	match := fakeProviderNotice.FindStringSubmatch(string(msg.Params))
	if match == nil {
		return
	}
	update := map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Warning: " + match[1] + "\n\n"}}
	if typed {
		update = map[string]any{"sessionUpdate": "session_info_update", "_meta": fakeFailureMeta(map[string]any{
			"id": "notice-1", "revision": 1, "category": "unknown", "severity": "warning", "title": match[1], "actions": []string{},
		})}
	}
	notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": update})
}

// fakeProviderFailure ends the prompt with a provider failure while the run a
// prompt started lasts, and reports whether it did.
func fakeProviderFailure(conn jsonrpc.MessageConn, msg *jsonrpc.Message, typed bool, turn int) bool {
	path := os.Getenv("JAZ_FAKE_ACP_FAILURE_FILE")
	if path == "" {
		return false
	}
	if match := fakeProviderFailures.FindStringSubmatch(string(msg.Params)); match != nil {
		if err := os.WriteFile(path, []byte(match[1]+" "+match[2]), 0o600); err != nil {
			panic(err)
		}
	}
	raw, err := os.ReadFile(path)
	fields := strings.Fields(string(raw))
	if err != nil || len(fields) != 2 {
		return false
	}
	left, _ := strconv.Atoi(fields[0])
	if left == 0 {
		return false
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d %s", left-1, fields[1])), 0o600); err != nil {
		panic(err)
	}
	kind := fakeFailureKinds[fields[1]]
	title := kind["title"].(string)
	if !typed {
		// codex-acp writes the error into the answer and ends the turn.
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
			"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": title + "\n\n"},
		}})
		sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
		return true
	}
	id := fmt.Sprintf("turn-%d:error", turn)
	if retrying, ok := kind["retrying"]; ok {
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
			"sessionUpdate": "session_info_update",
			"_meta":         fakeFailureMeta(map[string]any{"id": id, "revision": 1, "category": retrying, "severity": "warning", "title": "Reconnecting... 1/5", "actions": []string{}}),
		}})
	}
	sendResult(conn, msg, map[string]any{"stopReason": "end_turn", "_meta": fakeFailureMeta(map[string]any{
		"id": id, "revision": 2, "category": kind["category"], "severity": "error", "title": title, "actions": kind["actions"],
	})})
	return true
}

func fakeFailureMeta(failure map[string]any) map[string]any {
	return map[string]any{"jetbrains": map[string]any{"air": map[string]any{"version": 1, "sessionFailure": failure}}}
}

// fakeSteerFailure is the record a turn a native steer joined ends with when
// the steer starts a run of provider failures; codex-acp returns it on both
// the prompt response and the steer response.
func fakeSteerFailure(params json.RawMessage, typed bool) map[string]any {
	match := fakeProviderFailures.FindStringSubmatch(string(params))
	if match == nil || !typed {
		return nil
	}
	kind := fakeFailureKinds[match[2]]
	return fakeFailureMeta(map[string]any{
		"id": "steered-turn:error", "revision": 1, "category": kind["category"], "severity": "error", "title": kind["title"], "actions": kind["actions"],
	})
}
