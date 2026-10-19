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
var fakeProviderFailures = regexp.MustCompile(`provider fails (\d+) (overloaded|rejected|retried|reconnected|throttled|recovered|sidechat|noticed)`)

// fakeProviderNotice makes a prompt report a notice before it answers.
var fakeProviderNotice = regexp.MustCompile(`provider notice ([A-Za-z .]+)`)

// fakeFailureKinds are the records codex-acp and claude-agent-acp send for a
// failure the provider does not retry (overloaded), one the request itself
// causes (rejected), one the provider already retried (retried), a capacity
// failure after the provider retried its connection (reconnected), a rate
// limit the provider retried among other retries (throttled), an overload
// after the turn recovered from an earlier one (recovered), and an overload
// while a side chat on the session was retrying (sidechat), whose record
// codex-acp sends on the parent session with the side chat's scope, and an
// overload the provider retried with a notice among its retries (noticed). Retry
// warnings carry their own ids, as claude-agent-acp's do; codex-acp reuses
// the failure's.
var fakeFailureKinds = map[string]map[string]any{
	"overloaded":  {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}},
	"rejected":    {"category": "request", "title": "The requested model is not supported.", "actions": []string{}},
	"retried":     {"category": "service", "title": "API Error: Repeated 529 Overloaded errors.", "actions": []string{"retry"}, "retrying": []string{"service", "service"}},
	"reconnected": {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}, "retrying": []string{"connection"}},
	"throttled":   {"category": "limit", "title": "Rate limit reached.", "actions": []string{"retry"}, "retrying": []string{"limit", "connection"}},
	"recovered":   {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}, "retrying": []string{"service"}, "progress": true},
	"sidechat":    {"category": "service", "title": "Selected model is at capacity. Please try a different model.", "actions": []string{"retry"}, "retrying": []string{"service"}, "sideChat": true},
	"noticed":     {"category": "service", "title": "API Error: Repeated 529 Overloaded errors.", "actions": []string{"retry"}, "retrying": []string{"service", "unknown"}},
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
	retrying, _ := kind["retrying"].([]string)
	for n, category := range retrying {
		meta := fakeFailureMeta(map[string]any{"id": fmt.Sprintf("%s:retry:%d", fakeSessionID, n), "revision": 1, "category": category, "severity": "warning", "title": "Retrying...", "actions": []string{}})
		if kind["sideChat"] == true {
			meta["codex"] = map[string]any{"sideChat": map[string]any{"id": "side-1", "command": "side"}}
		}
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
			"sessionUpdate": "session_info_update", "_meta": meta,
		}})
	}
	if kind["progress"] == true {
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
			"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Looking into it. "},
		}})
	}
	sendResult(conn, msg, map[string]any{"stopReason": "end_turn", "_meta": fakeFailureMeta(map[string]any{
		"id": id, "revision": 1, "category": kind["category"], "severity": "error", "title": title, "actions": kind["actions"],
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
