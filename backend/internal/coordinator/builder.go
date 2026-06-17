package coordinator

import (
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/visualize"
)

type Builder struct {
	root          string
	workspace     string
	memoryRoot    string
	memoryEnabled func() bool
}

func NewBuilder(root, workspace, memoryRoot string, memoryEnabled func() bool) *Builder {
	return &Builder{root: root, workspace: workspace, memoryRoot: memoryRoot, memoryEnabled: memoryEnabled}
}

func (b *Builder) SystemPrompt() (string, error) {
	return b.SystemPromptForWorkspace(b.workspace)
}

func (b *Builder) SystemPromptForWorkspace(workspace string) (string, error) {
	return b.SystemPromptForWorkspaceSurface(workspace, visualize.SurfaceChat)
}

func (b *Builder) SystemPromptForWorkspaceSurface(workspace string, surface visualize.Surface) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = b.workspace
	}
	system, _, err := b.build(workspace, surface)
	return system, err
}

func (b *Builder) SkillsPrompt() (string, error) {
	_, skillsPrompt, err := b.build(b.workspace, visualize.SurfaceChat)
	return skillsPrompt, err
}

// ACPPrompt builds the prompt extension delivered to ACP agent sessions
// (codex, claude, grok). Unlike the coordinator prompt it carries no Jaz
// identity or delegation rules — agents keep their own system prompt and this
// is appended to it: runtime context, the user's standing rules (AGENTS.md),
// the jazmem memory horizons, and the skills catalog.
func (b *Builder) ACPPrompt(cwd string) (string, error) {
	return b.ACPPromptForArtifactSurface(cwd, string(visualize.SurfaceChat))
}

func (b *Builder) ACPPromptForArtifactSurface(cwd, surface string) (string, error) {
	catalog, err := skills.Load(b.root)
	if err != nil {
		return "", err
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = strings.TrimSpace(b.workspace)
	}
	if cwd == "" {
		cwd = "~/.jaz/workspaces/default"
	}
	now := time.Now()
	memoryRoot := b.memoryRoot
	if b.memoryEnabled != nil && !b.memoryEnabled() {
		memoryRoot = ""
	}
	return platformPrompt(b.root, cwd, memoryRoot, catalog.Prompt(), visualize.NormalizeSurface(surface), now)
}

func (b *Builder) build(workspace string, surface visualize.Surface) (system, skillsPrompt string, err error) {
	catalog, err := skills.Load(b.root)
	if err != nil {
		return "", "", err
	}
	skillsPrompt = catalog.Prompt()
	memoryRoot := b.memoryRoot
	if b.memoryEnabled != nil && !b.memoryEnabled() {
		memoryRoot = ""
	}
	system, err = prompt(b.root, workspace, memoryRoot, skillsPrompt, surface, time.Now())
	return system, skillsPrompt, err
}
