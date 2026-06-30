package coordinator

import (
	"context"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
	"github.com/wins/jaz/backend/internal/visualize"
)

type ConnectionSource interface {
	AgentConnections(context.Context) ([]connections.AgentConnection, error)
}

type AgentNameSource interface {
	EnabledAgentNames() ([]string, error)
}

type Builder struct {
	root          string
	workspace     string
	memoryRoot    string
	memoryEnabled func() bool
	connections   ConnectionSource
	agents        AgentNameSource
}

func NewBuilder(root, workspace, memoryRoot string, memoryEnabled func() bool) *Builder {
	return &Builder{root: root, workspace: workspace, memoryRoot: memoryRoot, memoryEnabled: memoryEnabled}
}

func (b *Builder) WithConnections(connections ConnectionSource) *Builder {
	b.connections = connections
	return b
}

func (b *Builder) WithAgents(agents AgentNameSource) *Builder {
	b.agents = agents
	return b
}

func (b *Builder) SystemPrompt() (string, error) {
	return b.SystemPromptForWorkspace(b.workspace)
}

func (b *Builder) SystemPromptForWorkspace(workspace string) (string, error) {
	return b.SystemPromptForWorkspaceSurface(workspace, visualize.SurfaceChat)
}

func (b *Builder) SystemPromptForWorkspaceSurface(workspace string, surface visualize.Surface) (string, error) {
	return b.SystemPromptForContext(context.Background(), workspace, surface)
}

func (b *Builder) SystemPromptForContext(ctx context.Context, workspace string, surface visualize.Surface) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = b.workspace
	}
	system, _, err := b.build(ctx, workspace, surface)
	return system, err
}

func (b *Builder) SkillsPrompt() (string, error) {
	_, skillsPrompt, err := b.build(context.Background(), b.workspace, visualize.SurfaceChat)
	return skillsPrompt, err
}

// ACPPrompt builds the prompt extension delivered to ACP agent sessions. Unlike
// the coordinator prompt it carries no Jaz identity — agents keep their own
// system prompt and this is appended to it: runtime context, Jaztools policy,
// the user's standing rules (AGENTS.md), connected-account paths, the jazmem
// memory horizons, and the skills catalog.
func (b *Builder) ACPPrompt(cwd string) (string, error) {
	return b.ACPPromptForArtifactSurface(cwd, string(visualize.SurfaceChat))
}

func (b *Builder) ACPPromptForArtifactSurface(cwd, surface string) (string, error) {
	return b.ACPPromptForContext(context.Background(), cwd, surface)
}

func (b *Builder) ACPPromptForContext(ctx context.Context, cwd, surface string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = strings.TrimSpace(b.workspace)
	}
	if cwd == "" {
		cwd = defaultWorkspace(b.root)
	}
	catalog, err := skills.LoadForWorkspace(b.root, cwd)
	if err != nil {
		return "", err
	}
	now := time.Now()
	memoryRoot := b.memoryRootForPrompt()
	connections, err := b.agentConnections(ctx, memoryRoot)
	if err != nil {
		return "", err
	}
	agents := b.agentNames()
	return platformPrompt(ctx, b.root, cwd, b.runtimeWorkspace(), memoryRoot, catalog.Prompt(), connections, agents, visualize.NormalizeSurface(surface), now)
}

func (b *Builder) PromptModulesForContext(ctx context.Context, opts acp.PromptModuleOptions) (promptmodule.Modules, error) {
	now := time.Now()
	memoryRoot := b.memoryRootForPrompt()
	memory, err := memoryData(memoryRoot, now)
	if err != nil {
		return nil, err
	}
	out := promptmodule.Modules{}
	if opts.Connections {
		connections, err := b.agentConnections(ctx, memoryRoot)
		if err != nil {
			return nil, err
		}
		prompt, err := jazplatform.RenderConnections(connections)
		if err != nil {
			return nil, err
		}
		out = out.Append(prompt)
	}
	prompt, err := jazplatform.RenderMemory(memory)
	if err != nil {
		return nil, err
	}
	return out.Append(prompt), nil
}

func (b *Builder) build(ctx context.Context, workspace string, surface visualize.Surface) (system, skillsPrompt string, err error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = b.workspace
	}
	catalog, err := skills.LoadForWorkspace(b.root, workspace)
	if err != nil {
		return "", "", err
	}
	skillsPrompt = catalog.Prompt()
	now := time.Now()
	memoryRoot := b.memoryRootForPrompt()
	connections, err := b.agentConnections(ctx, memoryRoot)
	if err != nil {
		return "", "", err
	}
	agents := b.agentNames()
	system, err = prompt(ctx, b.root, workspace, memoryRoot, skillsPrompt, connections, agents, surface, now)
	return system, skillsPrompt, err
}

func (b *Builder) memoryRootForPrompt() string {
	if b.memoryEnabled != nil && !b.memoryEnabled() {
		return ""
	}
	return b.memoryRoot
}

func (b *Builder) runtimeWorkspace() string {
	workspace := strings.TrimSpace(b.workspace)
	if workspace == "" {
		return defaultWorkspace(b.root)
	}
	return workspace
}

func (b *Builder) agentConnections(ctx context.Context, memoryRoot string) ([]connections.AgentConnection, error) {
	if b.connections == nil || strings.TrimSpace(memoryRoot) == "" {
		return nil, nil
	}
	return b.connections.AgentConnections(ctx)
}

func (b *Builder) agentNames() []string {
	if b.agents == nil {
		return nil
	}
	names, err := b.agents.EnabledAgentNames()
	if err != nil {
		return nil
	}
	return names
}
