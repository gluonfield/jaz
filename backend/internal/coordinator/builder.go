package coordinator

import (
	"sync"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/skills"
)

// Builder renders prompts on demand, re-reading skills and the prompt files
// (AGENTS.md, SOUL.md, ...) from disk on every call, so edits apply to the
// next turn without restarting the server. A rebuild that fails falls back to
// the last successful one rather than dropping the prompt mid-conversation.
type Builder struct {
	root string
	log  *log.Logger

	mu         sync.Mutex
	lastSystem string
	lastSkills string
}

func NewBuilder(root string, logger *log.Logger) *Builder {
	b := &Builder{root: root, log: logger}
	b.build() // warm the fallback so later disk errors degrade gracefully
	return b
}

// SystemPrompt returns the coordinator system prompt: runtime context with
// the current time, the prompt files, and the skills catalog.
func (b *Builder) SystemPrompt() string {
	system, _ := b.build()
	return system
}

// SkillsPrompt returns just the skills catalog block, used as the system
// prompt addition for spawned ACP agents.
func (b *Builder) SkillsPrompt() string {
	_, skillsPrompt := b.build()
	return skillsPrompt
}

func (b *Builder) build() (system, skillsPrompt string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	catalog, err := skills.Load(b.root)
	if err == nil {
		skillsPrompt = catalog.Prompt()
		system, err = Prompt(b.root, skillsPrompt)
	}
	if err != nil {
		if b.log != nil {
			b.log.Error("prompt rebuild failed, using last good build", "error", err)
		}
		return b.lastSystem, b.lastSkills
	}
	b.lastSystem, b.lastSkills = system, skillsPrompt
	return system, skillsPrompt
}
