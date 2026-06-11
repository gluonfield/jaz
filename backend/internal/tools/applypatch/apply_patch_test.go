package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/sessioncontext"
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

func TestApplyPatchRejectsEscapingAllowedRoots(t *testing.T) {
	tool := &Tool{Workspace: t.TempDir(), PathScope: AbsolutePaths}
	patch := `*** Begin Patch
*** Add File: ../outside.txt
+bad
*** End Patch`
	_, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "escapes allowed roots") {
		t.Fatalf("expected allowed roots escape error, got %v", err)
	}
}

func TestApplyPatchRejectsRelativeEscapeEvenWhenExtraRootWouldMatch(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &Tool{Workspace: workspace, ExtraRoots: []string{root}}
	patch := `*** Begin Patch
*** Add File: ../outside.txt
+bad
*** End Patch`
	_, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "escapes allowed roots") {
		t.Fatalf("expected allowed roots escape error, got %v", err)
	}
}

func TestApplyPatchUsesSessionCWD(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &Tool{Workspace: workspace}
	patch := `*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch`
	if _, err := tool.Execute(sessioncontext.WithCWD(context.Background(), cwd), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "hello.txt")); err != nil {
		t.Fatalf("patch did not apply inside session cwd: %v", err)
	}
}

func TestApplyPatchUsesSessionCWDOutsideWorkspace(t *testing.T) {
	cwd := t.TempDir()
	tool := &Tool{Workspace: t.TempDir()}
	patch := `*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch`
	if _, err := tool.Execute(sessioncontext.WithCWD(context.Background(), cwd), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "hello.txt")); err != nil {
		t.Fatalf("patch did not apply inside external session cwd: %v", err)
	}
}

func TestApplyPatchRejectsAbsolutePathOutsideBase(t *testing.T) {
	workspace := t.TempDir()
	tool := &Tool{Workspace: workspace}
	patch := `*** Begin Patch
*** Add File: ` + filepath.Join(t.TempDir(), "outside.txt") + `
+bad
*** End Patch`
	_, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "escapes allowed roots") {
		t.Fatalf("expected allowed roots escape error, got %v", err)
	}
}

func TestApplyPatchAllowsAbsolutePathsWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.txt")
	moved := filepath.Join(dir, "moved.txt")
	tool := &Tool{Workspace: t.TempDir(), PathScope: AbsolutePaths}

	patch := `*** Begin Patch
*** Add File: ` + target + `
+good
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}

	patch = `*** Begin Patch
*** Update File: ` + target + `
@@
-good
+better
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}

	patch = `*** Begin Patch
*** Update File: ` + target + `
*** Move to: ` + moved + `
@@
-better
+moved
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected moved source to be removed, stat err=%v", err)
	}
	data, err := os.ReadFile(moved)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "moved\n" {
		t.Fatalf("content = %q, want moved", data)
	}

	patch = `*** Begin Patch
*** Delete File: ` + moved + `
*** End Patch`
	if _, err := tool.Execute(context.Background(), map[string]any{"patch": patch}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(moved); !os.IsNotExist(err) {
		t.Fatalf("expected absolute delete to remove file, stat err=%v", err)
	}
}
