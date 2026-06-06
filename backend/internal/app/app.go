package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/tools"
	agentcancel "github.com/wins/jaz/backend/internal/tools/agent/cancel"
	agentlist "github.com/wins/jaz/backend/internal/tools/agent/list"
	agentsend "github.com/wins/jaz/backend/internal/tools/agent/send"
	agentspawn "github.com/wins/jaz/backend/internal/tools/agent/spawn"
	agentstatus "github.com/wins/jaz/backend/internal/tools/agent/status"
	agentwait "github.com/wins/jaz/backend/internal/tools/agent/wait"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"go.uber.org/fx"
)

type Config struct {
	Root      string
	Workspace string
	Provider  ProviderConfig
	ACP       acp.Config
}

type ProviderConfig struct {
	Type   string
	APIKey string
	Model  string
}

type Workspace string
type SkillsPrompt string
type SystemPrompt string

type Stores struct {
	fx.Out

	Store        *jsonstore.Store
	ACPStore     acp.Store
	StorageStore storage.Store
}

func NewStore(cfg Config) (Stores, error) {
	store, err := jsonstore.New(cfg.Root)
	if err != nil {
		return Stores{}, err
	}
	return Stores{Store: store, ACPStore: store, StorageStore: store}, nil
}

func NewWorkspace(cfg Config, store *jsonstore.Store) (Workspace, error) {
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = store.DefaultWorkspace()
	}
	return Workspace(workspace), os.MkdirAll(workspace, 0o755)
}

func LoadSkills(store *jsonstore.Store) (skills.Catalog, error) {
	return skills.Load(store.RootDir())
}

func NewSkillsPrompt(catalog skills.Catalog) SkillsPrompt {
	return SkillsPrompt(catalog.Prompt())
}

func NewSystemPrompt(store *jsonstore.Store, skillsPrompt SkillsPrompt) (SystemPrompt, error) {
	prompt, err := coordinator.Prompt(store.RootDir(), string(skillsPrompt))
	return SystemPrompt(prompt), err
}

func NewACPConfig(cfg Config, store *jsonstore.Store, workspace Workspace, skillsPrompt SkillsPrompt) acp.Config {
	cfg.ACP.Root = store.RootDir()
	cfg.ACP.Workspace = string(workspace)
	cfg.ACP.SystemPrompt = string(skillsPrompt)
	return cfg.ACP
}

func NewToolRegistry(commandManager *exectool.CommandManager, workspace Workspace, manager *acp.Manager) *tools.Registry {
	return tools.NewRegistry(
		&exectool.ExecCommandTool{Manager: commandManager, Workspace: string(workspace)},
		&exectool.WriteStdinTool{Manager: commandManager},
		&applypatch.Tool{Workspace: string(workspace)},
		&agentspawn.Tool{Manager: manager},
		&agentsend.Tool{Manager: manager},
		&agentstatus.Tool{Manager: manager},
		&agentwait.Tool{Manager: manager},
		&agentcancel.Tool{Manager: manager},
		&agentlist.Tool{Manager: manager},
	)
}

func NewProvider(cfg Config) (provider.Provider, error) {
	switch strings.ToLower(cfg.Provider.Type) {
	case "", "openai":
		if cfg.Provider.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires an API key; set OPENAI_API_KEY or openai.apikey")
		}
		return openaiprovider.New("https://api.openai.com/v1", cfg.Provider.APIKey, cfg.Provider.Model), nil
	case "openrouter":
		if cfg.Provider.APIKey == "" {
			return nil, fmt.Errorf("openrouter provider requires an API key; set OPENROUTER_API_KEY or openrouter.apikey")
		}
		return openaiprovider.New("https://openrouter.ai/api/v1", cfg.Provider.APIKey, cfg.Provider.Model), nil
	case "mock":
		return mockprovider.New(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q; valid providers are openai, openrouter, mock", cfg.Provider.Type)
	}
}

func NewAgent(cfg Config, modelProvider provider.Provider, registry *tools.Registry) *agent.Agent {
	return &agent.Agent{
		Provider: modelProvider,
		Model:    cfg.Provider.Model,
		Tools:    registry,
		MaxTurns: agent.DefaultMaxTurns,
	}
}

func ConnectACPCompletion(manager *acp.Manager, a *agent.Agent, store *jsonstore.Store, locks *sessionlock.Locks, events *sessionevents.Bus) {
	manager.Done = func(ctx context.Context, job acp.Job) {
		completeACP(ctx, a, store, locks, events, job)
	}
}

func completeACP(ctx context.Context, a *agent.Agent, store *jsonstore.Store, locks *sessionlock.Locks, events *sessionevents.Bus, job acp.Job) {
	if job.ParentID == "" {
		return
	}
	unlock := locks.Lock(job.ParentID)
	defer unlock()

	messages, err := store.LoadMessages(job.ParentID)
	if err != nil {
		return
	}
	messages = append(messages, provider.DeveloperMessage(acpCompletion(job)))
	ctx = sessioncontext.WithSessionID(ctx, job.ParentID)
	result, err := a.Complete(ctx, provider.Request{Messages: messages})
	if err != nil {
		content := fmt.Sprintf("ACP session %s finished, but coordinator follow-up failed: %v", job.Slug, err)
		messages = append(messages, provider.AssistantMessage(content, nil))
		_ = store.SaveMessages(job.ParentID, messages)
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acpEvent(job)})
		return
	}
	_ = store.SaveMessages(job.ParentID, result.Messages)
	if content := provider.MessageContent(result.Message); content != "" {
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acpEvent(job)})
	}
}

func acpCompletion(job acp.Job) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ACP session %s (%s) completed with state %s.", job.Slug, job.ACPAgent, job.State)
	if job.Error != "" {
		fmt.Fprintf(&b, "\nError: %s", job.Error)
	}
	if job.Assistant != "" {
		fmt.Fprintf(&b, "\nAssistant result:\n%s", job.Assistant)
	}
	return b.String()
}

func acpEvent(job acp.Job) *sessionevents.ACPEvent {
	calls := make([]sessionevents.ACPToolCall, 0, len(job.ToolCalls))
	for _, call := range job.ToolCalls {
		calls = append(calls, sessionevents.ACPToolCall{ID: call.ID, Title: call.Title, Status: call.Status})
	}
	return &sessionevents.ACPEvent{
		ID:         job.ID,
		Slug:       job.Slug,
		Title:      job.Title,
		ParentID:   job.ParentID,
		Agent:      job.ACPAgent,
		SessionID:  job.ACPSession,
		State:      job.State,
		StopReason: job.StopReason,
		Assistant:  job.Assistant,
		Error:      job.Error,
		ToolCalls:  calls,
	}
}
