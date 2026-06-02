package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	prompttemplate "github.com/wins/jaz/backend/internal/templates/coordinator"
)

var promptFiles = []string{"AGENTS.md", "SOUL.md", "HEARTBEAT.md"}

func Prompt(root, skillsPrompt string) (string, error) {
	return prompt(root, skillsPrompt, time.Now())
}

func prompt(root, skillsPrompt string, now time.Time) (string, error) {
	sections := make([]prompttemplate.Section, 0, len(promptFiles))
	for _, name := range promptFiles {
		content, err := readPromptFile(root, name)
		if err != nil {
			return "", err
		}
		if content != "" {
			sections = append(sections, prompttemplate.Section{Name: name, Content: content})
		}
	}
	return prompttemplate.Render(now, sections, skillsPrompt)
}

func readPromptFile(root, name string) (string, error) {
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
