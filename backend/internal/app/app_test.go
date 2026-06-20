package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
)

type appTestTool string

func (t appTestTool) Definition() tools.Definition {
	return tools.Function(string(t), "test tool", false, tools.ObjectSchema(nil, nil))
}

func (t appTestTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	return tools.Result{Content: "{}"}, nil
}

func TestNewToolRegistryAllowsApplyPatchAbsolutePaths(t *testing.T) {
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
		t.Fatal("apply_patch should allow absolute paths")
	}
}

func TestNewAgentDefersMCPToolsByRegistryGroup(t *testing.T) {
	registry := tools.NewRegistry(appTestTool("mcp_named_direct"))
	registry.SetGroup(mcpruntime.RegistryGroup, []tools.Tool{appTestTool("remote")})

	a := NewAgent(Config{}, nil, registry)
	if a.DeferTools("mcp_named_direct") {
		t.Fatal("direct tool with mcp_ prefix should not be deferred")
	}
	if !a.DeferTools("remote") {
		t.Fatal("tool in MCP registry group should be deferred")
	}
}

func TestNewRuntimeLayoutEnsuresDirsAndSkills(t *testing.T) {
	root := t.TempDir()

	layout, err := NewRuntimeLayout(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{layout.Root, layout.Sessions, layout.DefaultWorkspace, layout.UserSkills} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime dir %s missing: %v", dir, err)
		}
	}
	if entries, err := os.ReadDir(layout.UserSkills); err != nil || len(entries) != 0 {
		t.Fatalf("runtime layout should not install codebase skills: entries=%d err=%v", len(entries), err)
	}
	if _, err := os.Stat(filepath.Join(layout.Root, "system", "skills")); !os.IsNotExist(err) {
		t.Fatalf("system skills dir should not exist, err = %v", err)
	}
}

func TestDefaultSkillsManifestPin(t *testing.T) {
	if defaultSkillsManifestURL != "https://github.com/gluonfield/jaz-skills/releases/download/jaz-v0.0.28/manifest.json" {
		t.Fatalf("manifest url = %q", defaultSkillsManifestURL)
	}
	if defaultSkillsManifestSHA256 != "7acc2b360b5955721ae38edda1b75f5eb4409676a45dfad603e7325c3eab7497" {
		t.Fatalf("manifest sha = %q", defaultSkillsManifestSHA256)
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

func TestReloadableProviderReadsRuntimeEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	root := t.TempDir()

	loaded, err := NewProvider(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	reloadable := loaded.(*ReloadableProvider)
	router := reloadable.currentProvider().(*provider.Router)
	if _, ok := router.Provider[provider.ProviderOpenRouter].(provider.UnavailableProvider); !ok {
		t.Fatalf("openrouter should start unavailable: %#v", router.Provider[provider.ProviderOpenRouter])
	}

	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{"OPENROUTER_API_KEY": "runtime-key"}); err != nil {
		t.Fatal(err)
	}
	if err := reloadable.Reload(); err != nil {
		t.Fatal(err)
	}
	router = reloadable.currentProvider().(*provider.Router)
	openRouter, ok := router.Provider[provider.ProviderOpenRouter].(*openaiprovider.Provider)
	if !ok {
		t.Fatalf("openrouter provider = %T", router.Provider[provider.ProviderOpenRouter])
	}
	if openRouter.APIKey != "runtime-key" {
		t.Fatalf("openrouter key = %q", openRouter.APIKey)
	}
}
