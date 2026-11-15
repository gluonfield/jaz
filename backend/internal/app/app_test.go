package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpadapter"
	"github.com/wins/jaz/backend/internal/canvas"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/managedtool"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"github.com/wins/jaz/backend/internal/visualize"
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
		acp.NewManager(store, acp.Config{}, nil),
		store,
		sessionevents.New(),
		nil,
		visualize.NewService(store, store, sessionevents.New(), canvas.NewService(runtimefiles.New(store.RootDir()))),
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

func TestACPAgentConfigSourceUsesBundledNativeLogin(t *testing.T) {
	root := t.TempDir()
	adapters := acpadapter.New(root)
	status := adapters.Status(acp.AgentCodex)
	if status.Path == "" || status.BinDirs == "" {
		t.Fatalf("managed runtime paths unavailable: %#v", status)
	}
	dirs := filepath.SplitList(status.BinDirs)
	nativeDir := dirs[len(dirs)-1]
	executable := func(name string) string {
		if runtime.GOOS == "windows" {
			return name + ".exe"
		}
		return name
	}
	for _, path := range []string{status.Path, filepath.Join(nativeDir, executable("codex")), filepath.Join(nativeDir, executable("codex-code-mode-host"))} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := acp.BuiltinAgents()
	if _, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaultsFromCatalog(catalog)); err != nil {
		t.Fatal(err)
	}
	source := NewACPAgentConfigSource(store, catalog, managedtool.New(root), adapters)
	cfg, ok, err := source.(interface {
		MetadataConfig(string) (acp.AgentConfig, bool, error)
	}).MetadataConfig(acp.AgentCodex)
	if err != nil || !ok {
		t.Fatalf("native metadata config = %#v, %v", cfg, err)
	}
	login := acp.AgentLoginInvocationFor(acp.AgentCodex, root, cfg.Auth, cfg.LoginBinDir, cfg.Env)
	if !login.Available || login.Executable != filepath.Join(nativeDir, executable("codex")) {
		t.Fatalf("login bypassed bundled runtime: %#v", login)
	}
}

func TestNewRuntimeLayoutEnsuresDirsAndSkills(t *testing.T) {
	root := t.TempDir()

	layout, err := NewRuntimeLayout(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{layout.Root, layout.Sessions, layout.DefaultWorkspace, layout.UserSkills, layout.Ingest} {
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

func TestNewIntegrationRawWriterUsesRuntimeIngestRoot(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())

	writer := NewIntegrationRawWriter(layout)

	if writer.Root != layout.Ingest {
		t.Fatalf("raw writer root = %q, want %q", writer.Root, layout.Ingest)
	}
}

func TestTelegramProviderConfigUsesRuntimeEnvCredentials(t *testing.T) {
	t.Setenv(telegramconnector.EnvAppID, "")
	t.Setenv(telegramconnector.EnvAppHash, "")

	root := t.TempDir()
	if err := runtimeenv.Save(runtimeenv.Path(root), map[string]string{
		telegramconnector.EnvAppID:   "12345",
		telegramconnector.EnvAppHash: "test-hash",
	}); err != nil {
		t.Fatal(err)
	}

	cfg, ok, err := telegramProviderConfig(root)
	if err != nil || !ok || cfg.APIID != 12345 || cfg.APIHash != "test-hash" {
		t.Fatalf("config ok=%v cfg=%#v err=%v", ok, cfg, err)
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
	_, err = memory.Dream(context.Background(), jazmem.DreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "dream runner is not configured") {
		t.Fatalf("memory should require jaz's dream runner, got %v", err)
	}
	if strings.Contains(err.Error(), "OPENROUTER") {
		t.Fatalf("memory used provider-backed dream path: %v", err)
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

func TestMetadataConfigKeepsDisabledAgentsDisabled(t *testing.T) {
	t.Setenv("MUSE_CLI", "")
	t.Setenv("MUSE_INSTALL_DIR", "")
	root := t.TempDir()
	toolPath := managedtool.ExecutablePath(root, managedtool.MuseCLI)
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolPath, []byte("native launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := acp.BuiltinAgents()
	defaults := agentsettings.AgentDefaultsFromCatalog(catalog)
	if _, err := agentsettings.SaveAgentDefaults(store, defaults); err != nil {
		t.Fatal(err)
	}
	source := NewACPAgentConfigSource(store, catalog, managedtool.New(root), acpadapter.New(root))
	if _, enabled, err := source.AgentConfig(acp.AgentMuse); err != nil || enabled {
		t.Fatalf("disabled Muse runtime = %v, %v", enabled, err)
	}
	metadata := source.(interface {
		MetadataConfig(string) (acp.AgentConfig, bool, error)
	})
	cfg, ok, err := metadata.MetadataConfig(acp.AgentMuse)
	if err != nil || !ok || cfg.LoginBinDir != filepath.Dir(toolPath) {
		t.Fatalf("disabled Muse metadata = %#v, %v, %v", cfg, ok, err)
	}
	if _, enabled, err := source.AgentConfig(acp.AgentMuse); err != nil || enabled {
		t.Fatalf("metadata enabled Muse: %v, %v", enabled, err)
	}
}
