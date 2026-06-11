package runtimefiles

import (
	"os"
	"testing"
)

func TestEnsureCreatesRuntimeLayout(t *testing.T) {
	layout, err := Ensure(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		layout.Root,
		layout.Sessions,
		layout.Workspaces,
		layout.DefaultWorkspace,
		layout.UserSkills,
		layout.Automations,
		layout.ACPHome,
		layout.ACPCodexHome,
		layout.ACPTmp,
		layout.ACPNPMCache,
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime dir %s missing: %v", dir, err)
		}
	}
	for _, dir := range []string{layout.ACPHome, layout.ACPCodexHome, layout.ACPTmp, layout.ACPNPMCache} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("private runtime dir %s missing: %v", dir, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("private runtime dir %s mode = %o, want 700", dir, info.Mode().Perm())
		}
	}
}

func TestEnsureRejectsEmptyRoot(t *testing.T) {
	if _, err := Ensure(""); err == nil {
		t.Fatal("empty root accepted")
	}
}
