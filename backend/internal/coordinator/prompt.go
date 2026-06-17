package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/templates/jazagent"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
	"github.com/wins/jaz/backend/internal/visualize"
)

// PromptFiles are the agent prompt files read from the jaz root directory,
// in the order they are rendered into the coordinator system prompt.
var PromptFiles = []string{"AGENTS.md", "SOUL.md"}

func Prompt(root, workspace, memoryRoot, skillsPrompt string) (string, error) {
	return prompt(root, workspace, memoryRoot, skillsPrompt, visualize.SurfaceChat, time.Now())
}

// prompt joins the two layers: the Jaz agent prompt (identity and operating
// rules) and the platform prompt (runtime context, AGENTS.md, SOUL.md, memory,
// skills) that every agent in Jaz shares.
func prompt(root, workspace, memoryRoot, skillsPrompt string, surface visualize.Surface, now time.Time) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "~/.jaz/workspaces/default"
	}
	agentPrompt := jazagent.Render()
	platform, err := platformPrompt(root, workspace, memoryRoot, skillsPrompt, surface, now)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(agentPrompt, "\n") + "\n\n" + platform, nil
}

// platformPrompt renders the jaz extension shared by all agents: runtime
// context, prompt files, the memory protocol with live horizons, and the
// skills catalog.
func platformPrompt(root, cwd, memoryRoot, skillsPrompt string, surface visualize.Surface, now time.Time) (string, error) {
	// AGENTS.md and SOUL.md always render — an empty section tells every
	// agent the file exists and is editable, instead of silently vanishing.
	agents, err := ReadPromptFile(root, "AGENTS.md")
	if err != nil {
		return "", err
	}
	soul, err := ReadPromptFile(root, "SOUL.md")
	if err != nil {
		return "", err
	}
	memory, err := memoryData(memoryRoot, now)
	if err != nil {
		return "", err
	}
	return jazplatform.Render(jazplatform.Data{
		Agents:          orEmpty(agents),
		Date:            now.Format("January 2, 2006"),
		Time:            now.Format("15:04:05 MST"),
		Timezone:        now.Format("MST (UTCZ07:00)"),
		Weekday:         now.Format("Monday"),
		Human:           now.Format("Monday, January 2, 2006 at 15:04:05 MST"),
		Cwd:             strings.TrimSpace(cwd),
		Soul:            orEmpty(soul),
		ArtifactSurface: string(visualize.NormalizeSurface(string(surface))),
		Memory:          memory,
		Skills:          strings.TrimSpace(skillsPrompt),
	})
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
