package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
)

func TestNewToolRegistryAllowsNativeApplyPatchAbsolutePaths(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	registry := NewToolRegistry(
		exectool.NewCommandManager(),
		Workspace(t.TempDir()),
		nil,
		store,
		sessionevents.New(),
		nil,
	)
	tool, ok := registry.Get("apply_patch")
	if !ok {
		t.Fatal("apply_patch tool not registered")
	}
	patchTool, ok := tool.(*applypatch.Tool)
	if !ok {
		t.Fatalf("apply_patch tool = %T, want *applypatch.Tool", tool)
	}
	if patchTool.PathScope != applypatch.AbsolutePaths {
		t.Fatal("native apply_patch should allow absolute paths")
	}
}

func TestNewRuntimeLayoutEnsuresDirsAndManagedSkills(t *testing.T) {
	root := t.TempDir()

	layout, err := NewRuntimeLayout(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{layout.Root, layout.Sessions, layout.DefaultWorkspace, layout.UserSkills, layout.ManagedSkills} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime dir %s missing: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(layout.ManagedSkills, "jazmem", "SKILL.md")); err != nil {
		t.Fatalf("managed skill missing: %v", err)
	}
}

func TestNewMemoryDefaultsToRuntimeRoot(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())

	memory, err := NewMemory(Config{}, layout)
	if err != nil {
		t.Fatal(err)
	}
	defer memory.Close()

	if want := filepath.Join(layout.Root, "memory"); memory.Root() != want {
		t.Fatalf("memory root = %q, want %q", memory.Root(), want)
	}
	if want := filepath.Join(layout.Root, "jazmem.sqlite"); memory.DBPath() != want {
		t.Fatalf("memory db = %q, want %q", memory.DBPath(), want)
	}
	if _, err := os.Stat(filepath.Join(layout.Root, "memory", "LONG_TERM.md")); err != nil {
		t.Fatalf("memory horizons were not created: %v", err)
	}
}

func TestNewMemoryRespectsExplicitMemoryConfig(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	memoryRoot := filepath.Join(t.TempDir(), "memory")
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")

	memory, err := NewMemory(Config{Memory: MemoryConfig{Root: memoryRoot, DBPath: dbPath}}, layout)
	if err != nil {
		t.Fatal(err)
	}
	defer memory.Close()

	if memory.Root() != memoryRoot {
		t.Fatalf("memory root = %q, want %q", memory.Root(), memoryRoot)
	}
	if memory.DBPath() != dbPath {
		t.Fatalf("memory db = %q, want %q", memory.DBPath(), dbPath)
	}
}
