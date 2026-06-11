package coordinator

import (
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/skills"
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
	if strings.TrimSpace(workspace) == "" {
		workspace = b.workspace
	}
	system, _, err := b.build(workspace)
	return system, err
}

func (b *Builder) SkillsPrompt() (string, error) {
	_, skillsPrompt, err := b.build(b.workspace)
	return skillsPrompt, err
}

// ACPPrompt builds the prompt extension delivered to ACP agent sessions
// (codex, claude, grok). Unlike the coordinator prompt it carries no Jaz
// identity or delegation rules — agents keep their own system prompt and this
// is appended to it: the user's standing rules (AGENTS.md), the jazmem memory
// horizons, and the skills catalog.
func (b *Builder) ACPPrompt() (string, error) {
	catalog, err := skills.Load(b.root)
	if err != nil {
		return "", err
	}
	memoryRoot := b.memoryRoot
	if b.memoryEnabled != nil && !b.memoryEnabled() {
		memoryRoot = ""
	}
	return platformPrompt(b.root, memoryRoot, catalog.Prompt(), time.Now())
}

func (b *Builder) build(workspace string) (system, skillsPrompt string, err error) {
	catalog, err := skills.Load(b.root)
	if err != nil {
		return "", "", err
	}
	skillsPrompt = catalog.Prompt()
	memoryRoot := b.memoryRoot
	if b.memoryEnabled != nil && !b.memoryEnabled() {
		memoryRoot = ""
	}
	system, err = Prompt(b.root, workspace, memoryRoot, skillsPrompt)
	return system, skillsPrompt, err
}
