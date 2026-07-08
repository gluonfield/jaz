package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/managedtool"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
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

type completeACPTestProvider struct {
	called bool
}

func (p *completeACPTestProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	p.called = true
	return provider.Response{Message: provider.AssistantMessage("native follow-up", nil)}, nil
}

func (p *completeACPTestProvider) StreamComplete(context.Context, provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event)
	close(ch)
	return ch, nil
}

type completeACPParentStarter struct {
	sessionID string
	message   string
	err       error
}

func (s *completeACPParentStarter) StartInternalTurn(_ context.Context, sessionID, message string) error {
	s.sessionID = sessionID
	s.message = message
	return s.err
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

func TestACPAgentConfigSourcePassesManagedToolPathToAdapter(t *testing.T) {
	root := t.TempDir()
	toolPath := managedtool.ExecutablePath(root, managedtool.AntigravityCLI)
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolPath, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	defaults := agentsettings.AgentDefaultsFromCatalog(acp.BuiltinAgents())
	antigravity := defaults.ACP[acp.AgentAntigravity]
	antigravity.Enabled = true
	defaults.ACP[acp.AgentAntigravity] = antigravity
	if _, err := agentsettings.SaveAgentDefaults(store, defaults); err != nil {
		t.Fatal(err)
	}

	source := NewACPAgentConfigSource(store, acp.BuiltinAgents(), modelcatalog.NewService(nil), managedtool.New(root))
	cfg, ok, err := source.AgentConfig(acp.AgentAntigravity)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("antigravity config disabled")
	}
	if cfg.LoginBinDir != filepath.Dir(toolPath) {
		t.Fatalf("login bin dir = %q, want %q", cfg.LoginBinDir, filepath.Dir(toolPath))
	}
	if !slices.Contains(cfg.ManagedAdapterArgs, "--agy="+toolPath) {
		t.Fatalf("managed adapter args = %#v, want agy path", cfg.ManagedAdapterArgs)
	}
}

func TestWithManagedToolAdapterArgReplacesExistingValues(t *testing.T) {
	got := withManagedToolAdapterArg(
		[]string{"--auth=auto", "--agy", "/old/agy", "--dangerously-skip-permissions", "--agy=/stale/agy"},
		"--agy",
		"/new/agy",
	)
	want := []string{"--auth=auto", "--dangerously-skip-permissions", "--agy=/new/agy"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestCompleteACPStartsExternalParentFollowup(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	parent, err := store.CreateSession(storage.CreateSession{
		Slug:          "parent",
		Runtime:       storage.RuntimeACP,
		ModelProvider: acp.AgentClaude,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: acp.AgentClaude,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	fakeProvider := &completeACPTestProvider{}
	starter := &completeACPParentStarter{}

	completeACP(context.Background(), starter, &agent.Agent{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(),
		MaxTurns: 1,
	}, store, sessionlock.New(), nil, coordinator.NewBuilder(t.TempDir(), t.TempDir(), "", nil), log.New(io.Discard), acp.Job{
		ID:              "child-id",
		Slug:            "seed-deck",
		ACPAgent:        acp.AgentCodex,
		ModelProvider:   acp.AgentCodex,
		Model:           "gpt-5.5",
		ReasoningEffort: "xhigh",
		State:           acp.StateIdle,
		Assistant:       "Deck rebuilt at deck/physicslab-deck.html.",
		ParentID:        parent.ID,
	})

	if fakeProvider.called {
		t.Fatal("external ACP parent completion should not call native provider")
	}
	if starter.sessionID != parent.ID {
		t.Fatalf("internal turn session = %q, want parent session", starter.sessionID)
	}
	for _, want := range []string{
		"ACP session seed-deck (codex) completed with state idle.",
		"Deck rebuilt at deck/physicslab-deck.html.",
		"Continue from this result and report/update the user with relevant details:",
	} {
		if !strings.Contains(starter.message, want) {
			t.Fatalf("internal turn message %q missing %q", starter.message, want)
		}
	}
	messages, err := store.LoadMessages(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("message count = %d, want no synthetic parent message", len(messages))
	}
}

func TestCompleteACPDoesNotFakeExternalParentFollowupFailure(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	parent, err := store.CreateSession(storage.CreateSession{
		Slug:          "parent",
		Runtime:       storage.RuntimeACP,
		ModelProvider: acp.AgentClaude,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: acp.AgentClaude,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	events := sessionevents.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh := events.Subscribe(ctx, parent.ID)
	fakeProvider := &completeACPTestProvider{}
	starter := &completeACPParentStarter{err: errors.New("parent busy")}

	completeACP(ctx, starter, &agent.Agent{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(),
		MaxTurns: 1,
	}, store, sessionlock.New(), events, coordinator.NewBuilder(t.TempDir(), t.TempDir(), "", nil), log.New(io.Discard), acp.Job{
		ID:              "child-id",
		Slug:            "seed-deck",
		ACPAgent:        acp.AgentCodex,
		ModelProvider:   acp.AgentCodex,
		Model:           "gpt-5.5",
		ReasoningEffort: "xhigh",
		State:           acp.StateIdle,
		Assistant:       "Deck rebuilt at deck/physicslab-deck.html.",
		ParentID:        parent.ID,
	})

	if fakeProvider.called {
		t.Fatal("external ACP parent failure should not call native provider")
	}
	messages, err := store.LoadMessages(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("message count = %d, want no synthetic parent message", len(messages))
	}
	select {
	case event := <-eventCh:
		t.Fatalf("unexpected parent event %#v", event)
	default:
	}
	loaded, err := store.LoadSession(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusError || !strings.Contains(loaded.Error, "parent busy") {
		t.Fatalf("parent session status=%q error=%q, want parent busy error", loaded.Status, loaded.Error)
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

func TestDefaultSkillsManifestPin(t *testing.T) {
	if defaultSkillsManifestURL != "https://github.com/gluonfield/jaz-skills/releases/download/jaz-v0.0.95/manifest.json" {
		t.Fatalf("manifest url = %q", defaultSkillsManifestURL)
	}
	if defaultSkillsManifestSHA256 != "bd83441e3c4225f0ea8f282e18179ec4d25c28244ef5d1efe73462f8c9131e90" {
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
