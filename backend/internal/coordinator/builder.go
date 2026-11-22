package coordinator

import (
	"context"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/visualize"
)

type ConnectionSource interface {
	AgentConnections(context.Context) ([]connections.AgentConnection, error)
}

type AgentNameSource interface {
	EnabledAgentNames() ([]string, error)
}

type Builder struct {
	root           string
	workspace      string
	memoryRoot     string
	memoryEnabled  func() bool
	browserEnabled func() bool
	pluginFilter   func(string) bool
	connections    ConnectionSource
	agents         AgentNameSource
	instructions   instructions.Store
	cloud          *CloudScope
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

func (b *Builder) WithBrowserEnabled(enabled func() bool) *Builder {
	b.browserEnabled = enabled
	return b
}

func (b *Builder) WithPluginFilter(allow func(string) bool) *Builder {
	b.pluginFilter = allow
	return b
}

func (b *Builder) WithInstructions(store instructions.Store) *Builder {
	b.instructions = store
	return b
}

func (b *Builder) WithCloud(scope CloudScope) *Builder {
	copyScope := scope
	b.cloud = &copyScope
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

// ACPPrompt builds a thread extension without replacing the provider's identity.
// ACPPromptForContext selects the bot template for stored bot chats/group seats.
func (b *Builder) ACPPrompt(cwd string) (string, error) {
	return b.ACPPromptForArtifactSurface(cwd, string(visualize.SurfaceChat))
}

func (b *Builder) ACPPromptForArtifactSurface(cwd, surface string) (string, error) {
	return b.ACPPromptForContext(context.Background(), cwd, surface)
}

func (b *Builder) ACPPromptForContext(ctx context.Context, cwd, surface string) (string, error) {
	cloud := mergeCloudScope(b.cloud, ctx)
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = strings.TrimSpace(b.workspace)
	}
	if cwd == "" && cloud == nil {
		cwd = defaultWorkspace(b.root)
	}
	catalog, err := skills.LoadForWorkspaceFiltered(b.root, cwd, b.pluginFilter)
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
	runtimeWorkspace := ""
	if cloud == nil {
		runtimeWorkspace = b.runtimeWorkspace()
	}
	return platformPrompt(ctx, b.root, cwd, runtimeWorkspace, memoryRoot, catalog.PromptForContext(ctx), connections, agents, b.browserToolsEnabled(), visualize.NormalizeSurface(surface), now, b.instructions, cloud)
}

func (b *Builder) build(ctx context.Context, workspace string, surface visualize.Surface) (system, skillsPrompt string, err error) {
	cloud := mergeCloudScope(b.cloud, ctx)
	if strings.TrimSpace(workspace) == "" {
		workspace = b.workspace
	}
	catalog, err := skills.LoadForWorkspaceFiltered(b.root, workspace, b.pluginFilter)
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
	system, err = prompt(ctx, b.root, workspace, memoryRoot, skillsPrompt, connections, agents, b.browserToolsEnabled(), surface, now, b.instructions, cloud)
	return system, skillsPrompt, err
}

func (b *Builder) browserToolsEnabled() bool {
	return b.browserEnabled != nil && b.browserEnabled()
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
