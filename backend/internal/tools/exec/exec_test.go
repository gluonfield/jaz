package exec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExecCommandCompletes(t *testing.T) {
	manager := NewCommandManager()
	tool := &ExecCommandTool{Manager: manager, Workspace: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{
		"cmd":           "printf hello",
		"yield_time_ms": float64(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "completed" || payload["output"] != "hello" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestExecCommandWriteStdin(t *testing.T) {
	manager := NewCommandManager()
	execTool := &ExecCommandTool{Manager: manager, Workspace: t.TempDir()}
	result, err := execTool.Execute(context.Background(), map[string]any{
		"cmd":           "read line; printf \"got:%s\" \"$line\"",
		"yield_time_ms": float64(250),
	})
	if err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(result.Content), &first); err != nil {
		t.Fatal(err)
	}
	if first["status"] != "running" {
		t.Fatalf("expected running, got %#v", first)
	}

	writeTool := &WriteStdinTool{Manager: manager}
	result, err = writeTool.Execute(context.Background(), map[string]any{
		"session_id":    first["session_id"],
		"chars":         "jaz\n",
		"yield_time_ms": float64(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var second map[string]any
	if err := json.Unmarshal([]byte(result.Content), &second); err != nil {
		t.Fatal(err)
	}
	if second["status"] != "completed" || !strings.Contains(second["output"].(string), "got:jaz") {
		t.Fatalf("unexpected payload %#v", second)
	}
}
