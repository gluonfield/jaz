package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

type fakeNativeSteering struct {
	pending []*jsonrpc.Message
}

func (f *fakeNativeSteering) handle(t *testing.T, conn jsonrpc.MessageConn, msg, pending *jsonrpc.Message) (bool, *jsonrpc.Message) {
	response := os.Getenv("JAZ_FAKE_ACP_NATIVE_STEER_RESPONSE")
	if response == "" {
		return false, pending
	}
	var req struct {
		WaitForCompletion bool `json:"waitForCompletion"`
		Meta              struct {
			Steering struct {
				IdleBehavior string `json:"idleBehavior"`
			} `json:"steering"`
		} `json:"_meta"`
		Prompt []struct{ Text string } `json:"prompt"`
	}
	if err := json.Unmarshal(msg.Params, &req); err != nil || !req.WaitForCompletion {
		t.Fatal("expected completion-aware native steering")
	}
	text := string(msg.Params)
	if strings.Contains(text, "reject followup") {
		if strings.Contains(text, "after provider failure") && pending != nil {
			sendResult(conn, pending, map[string]any{"stopReason": "end_turn", "_meta": fakeSteerFailure(json.RawMessage(`"provider fails 1 rejected"`), true)})
			pending = nil
			time.Sleep(100 * time.Millisecond)
		}
		if response == "rpcError" {
			reply, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("native request rejected", nil))
			_ = conn.Send(context.Background(), reply)
		} else {
			sendResult(conn, msg, json.RawMessage(response))
		}
		return true, pending
	}
	if response != "nativeIdle" {
		return false, pending
	}
	if pending != nil {
		sendResult(conn, pending, map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 50, "outputTokens": 10, "totalTokens": 60}, "_meta": map[string]any{"usageId": "original-turn"}})
		pending = nil
	}
	if req.Meta.Steering.IdleBehavior == "promptRequired" {
		sendResult(conn, msg, map[string]any{"outcome": "promptRequired", "reason": "noRunningTurn"})
		return true, pending
	}
	f.pending = append(f.pending, msg)
	for _, block := range req.Prompt {
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"messageId":     "native-idle-reply",
			"content":       map[string]any{"type": "text", "text": block.Text + "\n"},
		}})
	}
	notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": map[string]any{
		"sessionUpdate": "usage_update", "used": 120, "size": 1000,
		"_meta": map[string]any{"usageId": "native-idle-turn", "usage": map[string]any{"inputTokens": 100, "outputTokens": 20, "totalTokens": 120}},
	}})
	if strings.Contains(text, "third") {
		for i, call := range f.pending {
			outcome := "injected"
			if i == 0 {
				outcome = "startedNewTurn"
			}
			sendResult(conn, call, map[string]any{"outcome": outcome, "stopReason": "end_turn"})
		}
		f.pending = nil
	}
	return true, pending
}
