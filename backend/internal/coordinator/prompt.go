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
var PromptFiles = []string{"AGENTS.md", "SOUL.md", "HEARTBEAT.md"}

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
