package acp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestWidgetPublishExtMethod(t *testing.T) {
	manager := NewManager(nil, Config{}, nil)
	manager.jobsByACP["acp-session"] = &Job{ID: "session-1", ACPSession: "acp-session"}
	var got WidgetPublishRequest
	manager.PublishWidget = func(req WidgetPublishRequest) (WidgetPublishResult, error) {
		got = req
		return WidgetPublishResult{WidgetID: "widget-1", Title: "Inbox", Version: 3, SizeHint: "2x2"}, nil
	}

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: ClientMethodWidgetPublish,
		Params: mustJSON(t, WidgetPublishRequest{SessionID: "acp-session", Title: "Inbox", HTML: "<p>hi</p>"}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	// The publisher receives the jaz session id, not the agent's ACP session id.
	if got.SessionID != "session-1" || got.HTML != "<p>hi</p>" {
		t.Fatalf("publish request = %#v", got)
	}
	var result WidgetPublishResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.WidgetID != "widget-1" || result.Version != 3 {
		t.Fatalf("publish result = %#v", result)
	}
}

func TestWidgetPublishExtMethodUnknownSession(t *testing.T) {
	manager := NewManager(nil, Config{}, nil)
	manager.PublishWidget = func(WidgetPublishRequest) (WidgetPublishResult, error) {
		t.Fatal("publisher should not be called for unknown sessions")
		return WidgetPublishResult{}, nil
	}
	_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: ClientMethodWidgetPublish,
		Params: mustJSON(t, WidgetPublishRequest{SessionID: "nope"}),
	})
	if rpcErr == nil {
		t.Fatal("expected error for unknown acp session")
	}
}

func TestWidgetPublishExtMethodUnconfigured(t *testing.T) {
	manager := NewManager(nil, Config{}, nil)
	_, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: ClientMethodWidgetPublish,
		Params: mustJSON(t, WidgetPublishRequest{SessionID: "acp-session"}),
	})
	if rpcErr == nil {
		t.Fatal("expected error when widget publishing is not configured")
	}
}
