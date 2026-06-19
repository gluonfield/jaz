package jazagent

import (
	"context"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
)

type ACPDependencies struct {
	Agent   *agent.Agent
	Store   Store
	Prompts PromptSource
	Log     *log.Logger
}

type ACPRunner struct {
	runner *Runner
}

func ACPAgentConfig() acp.AgentConfig {
	openRouter, _ := provider.ModelProviderByID(provider.ProviderOpenRouter)
	return acp.AgentConfig{
		Local:                   true,
		ProviderMode:            acp.AgentProviderModeAgentDefaults,
		ModelProviderCapability: provider.CapabilityJaz,
		ModelProvider:           provider.ProviderOpenRouter,
		Model:                   strings.TrimSpace(openRouter.DefaultModel),
		ReasoningEffort:         strings.TrimSpace(openRouter.DefaultReasoningEffort),
	}
}

func ACPAgentCatalog() acp.AgentCatalog {
	return acp.AgentCatalog{acp.AgentJaz: ACPAgentConfig()}
}

func NewACPRunner(deps ACPDependencies) ACPRunner {
	return ACPRunner{runner: &Runner{
		Agent:   deps.Agent,
		Store:   deps.Store,
		Prompts: deps.Prompts,
		Log:     deps.Log,
	}}
}

func RegisterACP(manager *acp.Manager, deps ACPDependencies) {
	manager.RegisterLocalAgent(acp.AgentJaz, NewACPRunner(deps))
}

func (r ACPRunner) Run(ctx context.Context, req acp.LocalAgentRequest) <-chan agent.StreamEvent {
	return r.runner.Run(ctx, Request{
		Session:                req.Session,
		Message:                req.Message,
		Attachments:            req.Attachments,
		PlanRequested:          req.PlanRequested,
		ArtifactSurface:        req.ArtifactSurface,
		SystemPromptExtensions: req.SystemPromptExtensions,
	})
}

func (r ACPRunner) RunUtility(ctx context.Context, req acp.LocalUtilityRequest) <-chan agent.StreamEvent {
	return r.runner.RunUtility(ctx, UtilityRequest{
		Session: req.Session,
		Message: req.Message,
	})
}
