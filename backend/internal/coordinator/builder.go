package coordinator

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/skills"
)

type Builder struct {
	root       string
	workspace  string
	memoryRoot string
}

func NewBuilder(root, workspace, memoryRoot string, _ *log.Logger) *Builder {
	return &Builder{root: root, workspace: workspace, memoryRoot: memoryRoot}
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

func (b *Builder) build(workspace string) (system, skillsPrompt string, err error) {
	catalog, err := skills.Load(b.root)
	if err != nil {
		return "", "", err
	}
	skillsPrompt = catalog.Prompt()
	system, err = Prompt(b.root, workspace, b.memoryRoot, skillsPrompt)
	return system, skillsPrompt, err
}
