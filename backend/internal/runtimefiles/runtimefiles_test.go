package runtimefiles

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureCreatesRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	layout, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if layout.IngestRaw != filepath.Join(root, "ingest", "raw") {
		t.Fatalf("ingest raw = %q, want %q", layout.IngestRaw, filepath.Join(root, "ingest", "raw"))
	}
	for _, dir := range []string{
		layout.Root,
		layout.Sessions,
		layout.Workspaces,
		layout.DefaultWorkspace,
		layout.UserSkills,
		layout.Automations,
		layout.Connections,
		filepath.Dir(layout.IngestRaw),
		layout.IngestRaw,
		layout.ACPCodexHome,
		layout.ACPClaudeConfig,
		layout.ACPOpenCodeConfig,
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime dir %s missing: %v", dir, err)
		}
	}
	for _, dir := range []string{
		layout.ACPCodexHome,
		layout.ACPClaudeConfig,
		layout.ACPOpenCodeConfig,
		layout.Connections,
		filepath.Dir(layout.IngestRaw),
		layout.IngestRaw,
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("private runtime dir %s missing: %v", dir, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("private runtime dir %s mode = %o, want 700", dir, info.Mode().Perm())
		}
	}
	for _, dir := range []string{"home", "tmp", "npm-cache"} {
		if _, err := os.Stat(filepath.Join(layout.Root, "acp", dir)); !os.IsNotExist(err) {
			t.Fatalf("generic ACP runtime dir %s should not be created, err = %v", dir, err)
		}
	}
}

func TestEnsureRejectsEmptyRoot(t *testing.T) {
	if _, err := Ensure(""); err == nil {
		t.Fatal("empty root accepted")
	}
}
