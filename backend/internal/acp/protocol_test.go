package acp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestRequestPermissionApprovesWorkspaceLocalTool(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{})
	manager.jobsByACP["acp-session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root}

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionRequestPermission,
		Params: mustJSON(t, acpschema.RequestPermissionRequest{
			SessionID: "acp-session",
			Options:   []acpschema.PermissionOption{{OptionID: "allow_once", Name: "Allow once"}},
			ToolCall: acpschema.ToolCallUpdate{
				Locations: []acpschema.ToolCallLocation{{Path: filepath.Join(root, "index.html")}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	var got map[string]map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["outcome"]["outcome"] != "selected" || got["outcome"]["optionId"] != "allow_once" {
		t.Fatalf("unexpected permission response: %s", raw)
	}
}

func TestRequestPermissionCancelsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(nil, Config{})
	manager.jobsByACP["acp-session"] = &Job{ID: "session", ACPSession: "acp-session", Cwd: root}

	raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
		Method: acpschema.ClientMethodSessionRequestPermission,
		Params: mustJSON(t, acpschema.RequestPermissionRequest{
			SessionID: "acp-session",
			Options:   []acpschema.PermissionOption{{OptionID: "allow_once", Name: "Allow once"}},
			ToolCall: acpschema.ToolCallUpdate{
				Locations: []acpschema.ToolCallLocation{{Path: filepath.Join(root, "..", "outside")}},
			},
		}),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}

	var got map[string]map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["outcome"]["outcome"] != "cancelled" {
		t.Fatalf("unexpected permission response: %s", raw)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
