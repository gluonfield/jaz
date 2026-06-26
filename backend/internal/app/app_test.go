package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/provider"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
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

func TestCompleteACPMaterializesExternalParentResult(t *testing.T) {
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

	completeACP(ctx, &agent.Agent{
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
		t.Fatal("external ACP parent completion should not call native provider")
	}
	loaded, err := store.LoadSession(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle || loaded.Error != "" {
		t.Fatalf("parent status = %s error=%q, want idle without error", loaded.Status, loaded.Error)
	}
	messages, err := store.LoadMessages(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	content := provider.MessageContent(messages[0])
	for _, want := range []string{
		"Child session seed-deck (codex) finished with state idle.",
		"Deck rebuilt at deck/physicslab-deck.html.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("completion message %q missing %q", content, want)
		}
	}
	select {
	case event := <-eventCh:
		if event.Type != "assistant" || event.Content != content {
			t.Fatalf("event = %#v, want assistant completion", event)
		}
		if event.ACP == nil || event.ACP.ModelProvider != acp.AgentCodex || event.ACP.Model != "gpt-5.5" || event.ACP.ReasoningEffort != "xhigh" {
			t.Fatalf("event acp metadata = %#v", event.ACP)
		}
	default:
		t.Fatal("missing parent completion event")
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
