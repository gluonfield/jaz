package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
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

func BuildAgent(cfg Config) (*agent.Agent, *jsonstore.Store, error) {
	store, err := jsonstore.New(cfg.Root)
	if err != nil {
		return nil, nil, err
	}
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = store.DefaultWorkspace()
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, nil, err
	}

	commandManager := exectool.NewCommandManager()
	acpManager := acp.NewManager(store, cfg.ACP)
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
		return nil, nil, err
	}

	return &agent.Agent{
		Provider: modelProvider,
		Model:    cfg.Provider.Model,
		Tools:    registry,
		MaxTurns: agent.DefaultMaxTurns,
	}, store, nil
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
