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

type Runtime struct {
	Agent        *agent.Agent
	Store        *jsonstore.Store
	ACP          *acp.Manager
	Locks        *sessionlock.Locks
	Events       *sessionevents.Bus
	SystemPrompt string
}

func BuildRuntime(cfg Config) (*Runtime, error) {
	store, err := jsonstore.New(cfg.Root)
	if err != nil {
		return nil, err
	}
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = store.DefaultWorkspace()
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}

	commandManager := exectool.NewCommandManager()
	catalog, err := skills.Load(store.RootDir())
	if err != nil {
		return nil, err
	}
	skillsPrompt := catalog.Prompt()
	systemPrompt, err := coordinator.Prompt(store.RootDir(), skillsPrompt)
	if err != nil {
		return nil, err
	}
	cfg.ACP.Root = store.RootDir()
	cfg.ACP.Workspace = workspace
	cfg.ACP.SystemPrompt = skillsPrompt
	acpManager := acp.NewManager(store, cfg.ACP)
	locks := sessionlock.New()
	events := sessionevents.New()
	registry := tools.NewRegistry(
		&exectool.ExecCommandTool{Manager: commandManager, Workspace: workspace},
		&exectool.WriteStdinTool{Manager: commandManager},
		&applypatch.Tool{Workspace: workspace},
		&agentspawn.Tool{Manager: acpManager},
		&agentsend.Tool{Manager: acpManager},
		&agentstatus.Tool{Manager: acpManager},
		&agentwait.Tool{Manager: acpManager},
		&agentcancel.Tool{Manager: acpManager},
		&agentlist.Tool{Manager: acpManager},
	)

	modelProvider, err := BuildProvider(cfg.Provider)
	if err != nil {
		return nil, err
	}

	runtime := &Runtime{Agent: &agent.Agent{
		Provider: modelProvider,
		Model:    cfg.Provider.Model,
		Tools:    registry,
		MaxTurns: agent.DefaultMaxTurns,
	}, Store: store, ACP: acpManager, Locks: locks, Events: events, SystemPrompt: systemPrompt}
	acpManager.Done = runtime.completeACP
	return runtime, nil
}

func (r *Runtime) completeACP(ctx context.Context, job acp.Job) {
	if job.ParentID == "" {
		return
	}
	unlock := r.Locks.Lock(job.ParentID)
	defer unlock()

	messages, err := r.Store.LoadMessages(job.ParentID)
	if err != nil {
		return
	}
	messages = append(messages, provider.DeveloperMessage(acpCompletion(job)))
	ctx = sessioncontext.WithSessionID(ctx, job.ParentID)
	result, err := r.Agent.Complete(ctx, provider.Request{Messages: messages})
	if err != nil {
		content := fmt.Sprintf("ACP session %s finished, but coordinator follow-up failed: %v", job.Slug, err)
		messages = append(messages, provider.AssistantMessage(content, nil))
		_ = r.Store.SaveMessages(job.ParentID, messages)
		r.Events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content})
		return
	}
	_ = r.Store.SaveMessages(job.ParentID, result.Messages)
	if content := provider.MessageContent(result.Message); strings.TrimSpace(content) != "" {
		r.Events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content})
	}
}

func acpCompletion(job acp.Job) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ACP session %s (%s) completed with state %s.", job.Slug, job.ACPAgent, job.State)
	if job.Error != "" {
		fmt.Fprintf(&b, "\nError: %s", job.Error)
	}
	if strings.TrimSpace(job.Assistant) != "" {
		fmt.Fprintf(&b, "\nAssistant result:\n%s", job.Assistant)
	}
	return b.String()
}

func BuildProvider(cfg ProviderConfig) (provider.Provider, error) {
	switch strings.ToLower(cfg.Type) {
	case "", "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires an API key; set OPENAI_API_KEY or openai.apikey")
		}
		return openaiprovider.New("https://api.openai.com/v1", cfg.APIKey, cfg.Model), nil
	case "openrouter":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openrouter provider requires an API key; set OPENROUTER_API_KEY or openrouter.apikey")
		}
		return openaiprovider.New("https://openrouter.ai/api/v1", cfg.APIKey, cfg.Model), nil
	case "mock":
		return mockprovider.New(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q; valid providers are openai, openrouter, mock", cfg.Type)
	}
}
