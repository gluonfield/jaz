package app

import (
	"testing"

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
