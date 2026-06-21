package exec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/sessioncontext"
)

func TestExecCommandCompletes(t *testing.T) {
	manager := NewCommandManager()
	tool := &ExecCommandTool{Manager: manager, Workspace: t.TempDir()}
	cmd := "printf hello"
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"cmd":           cmd,
		"yield_time_ms": float64(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "completed" || strings.TrimSpace(payload["output"].(string)) != "hello" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestExecCommandDefaultsToSessionCWD(t *testing.T) {
	manager := NewCommandManager()
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &ExecCommandTool{Manager: manager, Workspace: workspace}
	cmd := "pwd"
	if runtime.GOOS == "windows" {
		cmd = "cd"
	}
	result, err := tool.Execute(sessioncontext.WithCWD(context.Background(), cwd), map[string]any{
		"cmd":           cmd,
		"yield_time_ms": float64(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(payload["output"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pwd output = %#v, want %q", payload["output"], cwd)
	}
}

func TestExecCommandUsesSessionCWDOutsideWorkspace(t *testing.T) {
	manager := NewCommandManager()
	cwd := t.TempDir()
	tool := &ExecCommandTool{Manager: manager, Workspace: t.TempDir()}
	cmd := "pwd"
	if runtime.GOOS == "windows" {
		cmd = "cd"
	}
	result, err := tool.Execute(sessioncontext.WithCWD(context.Background(), cwd), map[string]any{
		"cmd":           cmd,
		"yield_time_ms": float64(1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(payload["output"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pwd output = %#v, want %q", payload["output"], cwd)
	}
}

func TestExecCommandWriteStdin(t *testing.T) {
	manager := NewCommandManager()
	execTool := &ExecCommandTool{Manager: manager, Workspace: t.TempDir()}
	inputs := map[string]any{
		"cmd":           `read line; printf "got:%s" "$line"`,
		"yield_time_ms": float64(250),
	}
	if runtime.GOOS == "windows" {
		inputs["shell"] = "powershell.exe"
		inputs["cmd"] = `$line = [Console]::In.ReadLine(); Write-Output "got:$line"`
	}
	result, err := execTool.Execute(context.Background(), inputs)
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
