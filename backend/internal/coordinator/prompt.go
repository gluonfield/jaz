package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	prompttemplate "github.com/wins/jaz/backend/internal/templates/coordinator"
)

// PromptFiles are the agent prompt files read from the jaz root directory,
// in the order they are rendered into the coordinator system prompt.
var PromptFiles = []string{"AGENTS.md", "SOUL.md"}

func Prompt(root, workspace, memoryRoot, skillsPrompt string) (string, error) {
	return prompt(root, workspace, memoryRoot, skillsPrompt, time.Now())
}

func prompt(root, workspace, memoryRoot, skillsPrompt string, now time.Time) (string, error) {
	sections := make([]prompttemplate.Section, 0, len(PromptFiles))
	for _, name := range PromptFiles {
		content, err := ReadPromptFile(root, name)
		if err != nil {
			return "", err
		}
		if content != "" {
			sections = append(sections, prompttemplate.Section{Name: name, Body: content})
		}
	}
	memory, err := memorySections(memoryRoot, now)
	if err != nil {
		return "", err
	}
	sections = append(sections, memory...)
	if strings.TrimSpace(workspace) == "" {
		workspace = "~/.jaz/workspaces/default"
	}
	return prompttemplate.Render(now, workspace, sections, skillsPrompt)
}

// acpPromptHeader frames the sections appended to an ACP agent's own system
// prompt; it must not restate the coordinator identity or delegation rules.
const acpPromptHeader = "The user runs you through Jaz, their personal assistant. " +
	"The sections below extend your instructions: the user's standing rules (AGENTS.md), " +
	"their persistent memory (memory/*), and Jaz skills. Follow the memory rules — record " +
	"durable facts, decisions, and the user's goals as you learn them."

// acpPrompt composes the system prompt extension for ACP agent sessions:
// AGENTS.md plus the jazmem horizons and the skills catalog. SOUL.md stays
// coordinator-only.
func acpPrompt(root, memoryRoot, skillsPrompt string, now time.Time) (string, error) {
	var sections []prompttemplate.Section
	agents, err := ReadPromptFile(root, "AGENTS.md")
	if err != nil {
		return "", err
	}
	if agents != "" {
		sections = append(sections, prompttemplate.Section{Name: "AGENTS.md", Body: agents})
	}
	memory, err := memorySections(memoryRoot, now)
	if err != nil {
		return "", err
	}
	sections = append(sections, memory...)
	skillsPrompt = strings.TrimSpace(skillsPrompt)
	if len(sections) == 0 && skillsPrompt == "" {
		return "", nil
	}
	var sb strings.Builder
	sb.WriteString(acpPromptHeader)
	for _, section := range sections {
		sb.WriteString("\n\n## ")
		sb.WriteString(section.Name)
		sb.WriteString("\n\n")
		sb.WriteString(section.Body)
	}
	if skillsPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(skillsPrompt)
	}
	return sb.String(), nil
}

// ReadPromptFile reads a single prompt file from root, returning "" when the
// file does not exist or root is unset.
func ReadPromptFile(root, name string) (string, error) {
	if root == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
