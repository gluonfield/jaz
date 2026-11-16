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
	delayed *jsonrpc.Message
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
	}
	if err := json.Unmarshal(msg.Params, &req); err != nil || !req.WaitForCompletion || req.Meta.Steering.IdleBehavior != "promptRequired" {
		t.Fatal("expected completion-aware steering with host-owned idle fallback")
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
	if !strings.Contains(text, "idle followup") {
		return false, pending
	}
	if pending != nil {
		sendResult(conn, pending, map[string]any{"stopReason": "end_turn"})
		pending = nil
	}
	if strings.Contains(text, "one") {
		f.delayed = msg
		return true, pending
	}
	result := map[string]any{"outcome": "promptRequired", "reason": "noRunningTurn"}
	sendResult(conn, msg, result)
	if f.delayed != nil {
		time.Sleep(100 * time.Millisecond)
		sendResult(conn, f.delayed, result)
		f.delayed = nil
	}
	return true, pending
}
