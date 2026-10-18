package acp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSpawnConfigDefaultsWidgetSurfaceToWidgetMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{
		ACPAgent:               "fake",
		ArtifactSurface:        " widget ",
		SystemPromptExtensions: []string{" run context ", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.ArtifactSurface != "widget" {
		t.Fatalf("artifact surface = %q", req.ArtifactSurface)
	}
	if req.MCPServerPolicy != MCPServerPolicyWidget {
		t.Fatalf("mcp server policy = %q, want %q", req.MCPServerPolicy, MCPServerPolicyWidget)
	}
	if len(req.SystemPromptExtensions) != 1 || req.SystemPromptExtensions[0] != "run context" {
		t.Fatalf("system prompt extensions = %#v", req.SystemPromptExtensions)
	}
}

func TestSpawnConfigPreservesExplicitMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{
		ACPAgent:        "fake",
		ArtifactSurface: "widget",
		MCPServerPolicy: MCPServerPolicyMemorySearchWorker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.MCPServerPolicy != MCPServerPolicyMemorySearchWorker {
		t.Fatalf("mcp server policy = %q", req.MCPServerPolicy)
	}
}

func TestSpawnConfigRejectsJazAgent(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		AgentJaz: AgentConfig{Local: true},
	}}
	_, _, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentJaz})
	if err == nil || !strings.Contains(err.Error(), `acp agent "jaz" is not selectable`) {
		t.Fatalf("err = %v", err)
	}
}

func TestSpawnConfigRejectsProviderWithoutRequiredWireProtocol(t *testing.T) {
	manager := &Manager{
		cfg: Config{Providers: map[string]modelprovider.ModelProviderConfig{
			"internal": {
				Type:         "openai-compatible",
				BaseURL:      "https://llm.internal/v1",
				Capabilities: []string{modelprovider.CapabilityChatCompletions},
			},
		}},
		agents: AgentCatalog{
			AgentCodex: {
				Command:                 AgentCodex,
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: modelprovider.CapabilityResponses,
				ModelProvider:           "internal",
			},
		},
	}
	_, _, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentCodex})
	if err == nil || !strings.Contains(err.Error(), `model provider "internal" does not support "responses"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestSpawnConfigDefaultsToCodexBeforeClaude(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		AgentClaude: AgentConfig{Command: "claude"},
		AgentCodex:  AgentConfig{Command: "codex"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if req.ACPAgent != AgentCodex {
		t.Fatalf("default agent = %q", req.ACPAgent)
	}
}

func TestSpawnConfigDefaultsWorkerSourceToRestrictedMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	cases := []struct {
		source string
		want   string
	}{
		{storage.SourceMemorySearch, MCPServerPolicyMemorySearchWorker},
		{storage.SourceMemorySource, MCPServerPolicyMemorySourceWorker},
		{storage.SourceMemoryDream, MCPServerPolicyMemorySourceWorker},
		{storage.LegacySourceBrowserTask, MCPServerPolicyRetiredWorker},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			req, _, _, err := manager.spawnConfig(SpawnRequest{
				ACPAgent:   "fake",
				SourceType: tc.source,
			})
			if err != nil {
				t.Fatal(err)
			}
			if req.MCPServerPolicy != tc.want {
				t.Fatalf("mcp server policy = %q, want %q", req.MCPServerPolicy, tc.want)
			}
		})
	}
}

func TestLegacyBrowserWorkerPolicyIsRetiredOnResume(t *testing.T) {
	session := storage.Session{
		SourceType: storage.LegacySourceBrowserTask,
		RuntimeRef: &storage.RuntimeRef{MCPServerPolicy: "browser_worker"},
	}
	if got := effectiveMCPServerPolicy(session); got != MCPServerPolicyRetiredWorker {
		t.Fatalf("policy = %q, want %q", got, MCPServerPolicyRetiredWorker)
	}
}

func TestSpawnConfigReasoningEffortNoneAndDefault(t *testing.T) {
	agents := []string{AgentCodex, AgentClaude, AgentGrok, AgentOpenCode}
	catalog := AgentCatalog{}
	for _, agent := range agents {
		catalog[agent] = AgentConfig{Command: agent, Model: "gpt-5/high", ReasoningEffort: "high"}
	}
	manager := &Manager{agents: catalog}

	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			_, cfg, effort, err := manager.spawnConfig(SpawnRequest{ACPAgent: agent, ReasoningEffort: "none"})
			if err != nil {
				t.Fatal(err)
			}
			wantModel := "gpt-5/high"
			if agent == AgentCodex {
				wantModel = "gpt-5"
			}
			if cfg.Model != wantModel {
				t.Fatalf("model = %q, want %q", cfg.Model, wantModel)
			}
			if effort != "" || cfg.ReasoningEffort != "" {
				t.Fatalf("effort = %q, cfg effort = %q; want no reasoning effort", effort, cfg.ReasoningEffort)
			}
			_, cfg, effort, err = manager.spawnConfig(SpawnRequest{ACPAgent: agent})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Model != "gpt-5/high" {
				t.Fatalf("model = %q, want configured default model", cfg.Model)
			}
			if effort != "high" || cfg.ReasoningEffort != "high" {
				t.Fatalf("effort = %q, cfg effort = %q; want configured default high", effort, cfg.ReasoningEffort)
			}
		})
	}
}

func TestSpawnConfigUsesCodexOpenAIDefaultModelForOpenAIProviders(t *testing.T) {
	tests := []struct {
		name               string
		configuredProvider string
		requestProvider    string
		wantProvider       string
	}{
		{
			name:               "api key",
			configuredProvider: modelprovider.ProviderOpenAI,
			requestProvider:    CodexProviderOpenAIAPIKey,
			wantProvider:       CodexProviderOpenAIAPIKey,
		},
		{
			name:               "oauth",
			configuredProvider: modelprovider.ProviderOpenRouter,
			requestProvider:    modelprovider.ProviderOpenAI,
			wantProvider:       modelprovider.ProviderOpenAI,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{agents: AgentCatalog{
				AgentCodex: AgentConfig{
					Command:       AgentCodex,
					ProviderMode:  AgentProviderModeAgentDefaults,
					ModelProvider: tt.configuredProvider,
				},
			}}
			_, cfg, _, err := manager.spawnConfig(SpawnRequest{
				ACPAgent:      AgentCodex,
				ModelProvider: tt.requestProvider,
			})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ModelProvider != tt.wantProvider || cfg.Model != CodexOpenAIDefaultModel {
				t.Fatalf("unexpected codex provider override %#v", cfg)
			}
		})
	}
}

func TestSpawnConfigUsesConfiguredProviderDefaultModel(t *testing.T) {
	manager := &Manager{
		cfg: Config{Providers: map[string]modelprovider.ModelProviderConfig{
			"acme": {
				Type:         "openai-compatible",
				BaseURL:      "https://acme.test/v1",
				DefaultModel: "acme-coder",
				Capabilities: []string{modelprovider.CapabilityChatCompletions},
			},
		}},
		agents: AgentCatalog{AgentOpenCode: {
			Command:                 AgentOpenCode,
			ProviderMode:            AgentProviderModeAgentDefaults,
			ModelProviderCapability: modelprovider.CapabilityChatCompletions,
			ModelProvider:           modelprovider.ProviderOpenRouter,
			Model:                   modelprovider.DefaultOpenRouterModel,
		}},
	}
	_, cfg, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentOpenCode, ModelProvider: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelProvider != "acme" || cfg.Model != "acme-coder" {
		t.Fatalf("provider switch = %#v", cfg)
	}
}

func TestSpawnConfigRejectsOpenAIAPIKeyWithoutResponsesCapability(t *testing.T) {
	manager := &Manager{
		cfg: Config{Providers: map[string]modelprovider.ModelProviderConfig{
			modelprovider.ProviderOpenAI: {Capabilities: []string{modelprovider.CapabilityChatCompletions}},
		}},
		agents: AgentCatalog{AgentCodex: {
			Command:                 AgentCodex,
			ProviderMode:            AgentProviderModeAgentDefaults,
			ModelProviderCapability: modelprovider.CapabilityResponses,
			ModelProvider:           modelprovider.ProviderOpenAI,
		}},
	}
	_, _, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentCodex, ModelProvider: CodexProviderOpenAIAPIKey})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("Chat-only OpenAI API key override error = %v", err)
	}
}

func TestSpawnConfigResolvesModelLabelsWithinConfiguredProvider(t *testing.T) {
	manager := &Manager{
		cfg: Config{ModelCatalog: modelcatalog.NewService(nil)},
		agents: AgentCatalog{
			AgentOpenCode: {
				Command:                 AgentOpenCode,
				ProviderMode:            AgentProviderModeAgentDefaults,
				ModelProviderCapability: modelprovider.CapabilityChatCompletions,
				ModelProvider:           modelprovider.ProviderOpenAI,
				Model:                   modelprovider.DefaultOpenAIModel,
			},
		},
	}

	_, cfg, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: AgentOpenCode, Model: "GPT-6 Sol"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != modelprovider.OpenAIModelGPT6Sol {
		t.Fatalf("model label resolved outside configured provider: %#v", cfg)
	}
}

func TestSpawnConfigRejectsModelSpecificUnsupportedReasoning(t *testing.T) {
	for _, input := range []struct{ agent, model string }{
		{AgentClaude, "sonnet"},
		{AgentCodex, modelprovider.OpenAIModelGPT6Sol},
	} {
		manager := &Manager{
			cfg: Config{ModelCatalog: warmOpenRouterCatalog(t)},
			agents: AgentCatalog{
				input.agent: {Command: input.agent, Model: input.model, ReasoningEffort: "minimal"},
			},
		}
		_, _, _, err := manager.spawnConfig(SpawnRequest{ACPAgent: input.agent})
		if err == nil || !strings.Contains(err.Error(), `reasoning effort "minimal" is not supported for `+input.agent+` model "`+input.model+`"`) {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestABotsWorkersAndGroupThreadsRunLikeTheBot(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	fake := AgentConfig{Command: "fake"}
	manager := NewManager(store, Config{
		Root:         t.TempDir(),
		Workspace:    t.TempDir(),
		Agents:       map[string]AgentConfig{AgentClaude: fake},
		ModelCatalog: modelcatalog.NewService(nil),
	}, log.New(io.Discard))
	ctx := t.Context()
	bot, err := manager.CreateSession(ctx, SpawnRequest{ACPAgent: AgentClaude, Slug: "ceo", ReasoningEffort: "high", Home: t.TempDir(), SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if bot.ModelProvider != AgentClaude {
		t.Fatalf("bot stored provider %q, want the agent's own name so the case is covered", bot.ModelProvider)
	}
	owed, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "research"})
	if err != nil {
		t.Fatal(err)
	}
	var member storage.Session
	for _, req := range []SpawnRequest{
		{ParentID: bot.ID, Slug: "research", Commitment: owed.ID},
		{Slug: "ceo in team", SourceType: storage.SourceBotMember, SourceID: bot.ID},
	} {
		thread, err := manager.CreateSession(ctx, req)
		if err != nil {
			t.Fatalf("%s thread: %v", req.Slug, err)
		}
		if thread.RuntimeRef.Agent != AgentClaude || thread.Model != bot.Model || thread.ReasoningEffort != "high" || thread.RuntimeRef.Cwd != bot.RuntimeRef.Cwd || thread.SourceID != bot.ID {
			t.Fatalf("%s thread = %+v, want it to run like bot %+v", req.Slug, thread, bot)
		}
		member = thread
	}
	if bot.RuntimeRef.MCPServerPolicy != MCPServerPolicyBot || member.RuntimeRef.MCPServerPolicy != MCPServerPolicyBot {
		t.Fatalf("bot tools: bot %q, group thread %q", bot.RuntimeRef.MCPServerPolicy, member.RuntimeRef.MCPServerPolicy)
	}
}

func TestThreadsABotStartsAreItsSubtasks(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	fake := AgentConfig{Command: "fake"}
	manager := NewManager(store, Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		Agents:    map[string]AgentConfig{"alpha": fake, "fake": fake},
	}, log.New(io.Discard))
	ctx := context.Background()
	bot, err := manager.CreateSession(ctx, SpawnRequest{ACPAgent: "fake", Slug: "scout", Model: "scout-model", ReasoningEffort: "low", Home: t.TempDir(), SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := manager.CreateSession(ctx, SpawnRequest{Slug: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.RuntimeRef.Agent != "alpha" {
		t.Fatalf("default agent = %q, want alpha so inheritance is observable", plain.RuntimeRef.Agent)
	}

	owed, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "research"})
	if err != nil {
		t.Fatal(err)
	}
	subtask, err := manager.CreateSession(ctx, SpawnRequest{ParentID: bot.ID, Slug: "research", Commitment: owed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if subtask.SourceType != storage.SourceBotWorker || subtask.SourceID != bot.ID || subtask.RuntimeRef.Cwd != bot.RuntimeRef.Cwd {
		t.Fatalf("subtask = %+v, want a bot_worker of %s in %s", subtask, bot.ID, bot.RuntimeRef.Cwd)
	}
	if subtask.RuntimeRef.Agent != "fake" || subtask.Model != bot.Model || subtask.ReasoningEffort != bot.ReasoningEffort {
		t.Fatalf("subtask runs %s/%s/%s, want the bot's fake/%s/%s", subtask.RuntimeRef.Agent, subtask.Model, subtask.ReasoningEffort, bot.Model, bot.ReasoningEffort)
	}
	other, err := manager.CreateSession(ctx, SpawnRequest{ParentID: bot.ID, Slug: "elsewhere", ACPAgent: "alpha", Commitment: owed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if other.SourceType != storage.SourceBotWorker || other.RuntimeRef.Agent != "alpha" || other.Model == bot.Model {
		t.Fatalf("subtask on another agent = %+v", other)
	}
	child, err := manager.CreateSession(ctx, SpawnRequest{ParentID: plain.ID, Slug: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if child.SourceType != "" || child.RuntimeRef.Cwd == plain.RuntimeRef.Cwd {
		t.Fatalf("a chat's child became a subtask: %+v", child)
	}
}
