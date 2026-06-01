package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchAddUpdateDelete(t *testing.T) {
	workspace := t.TempDir()
	tool := &Tool{Workspace: workspace}

	add := `*** Begin Patch
*** Add File: hello.txt
+hello
+world
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": add}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	update := `*** Begin Patch
*** Update File: hello.txt
@@
 hello
-world
+jaz
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": update}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "hello\njaz\n" {
		t.Fatalf("unexpected file content %q", got)
	}

	del := `*** Begin Patch
*** Delete File: hello.txt
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": del}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
}

func TestApplyPatchRejectsEscapingWorkspace(t *testing.T) {
	tool := &Tool{Workspace: t.TempDir()}
	patch := `*** Begin Patch
*** Add File: ../outside.txt
+bad
*** End Patch`
	_, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected workspace escape error, got %v", err)
	}
}
