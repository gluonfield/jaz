package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/templates/jazagent"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
	"github.com/wins/jaz/backend/internal/visualize"
)

// PromptFiles are the agent prompt files read from the jaz root directory,
// in the order they are rendered into the coordinator system prompt.
var PromptFiles = []string{"AGENTS.md", "SOUL.md", "INTERNAL.md"}

func Prompt(root, workspace, memoryRoot, skillsPrompt string) (string, error) {
	return prompt(context.Background(), root, workspace, memoryRoot, skillsPrompt, nil, nil, visualize.SurfaceChat, time.Now())
}

// prompt joins the two layers: the Jaz agent prompt (identity and operating
// rules) and the platform prompt (runtime context, AGENTS.md, SOUL.md,
// INTERNAL.md, memory, skills) that every agent in Jaz shares.
func prompt(ctx context.Context, root, workspace, memoryRoot, skillsPrompt string, connections []connections.AgentConnection, agentNames []string, surface visualize.Surface, now time.Time) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = defaultWorkspace(root)
	}
	agentPrompt, err := jazagent.Render()
	if err != nil {
		return "", err
	}
	platform, err := platformPrompt(ctx, root, workspace, workspace, memoryRoot, skillsPrompt, connections, agentNames, surface, now)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(agentPrompt, "\n") + "\n\n" + platform, nil
}

func defaultWorkspace(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return "~/.jaz/workspaces/default"
	}
	return filepath.Join(root, "workspaces", "default")
}

// platformPrompt renders the jaz extension shared by all agents: runtime
// context, AGENTS.md, SOUL.md, INTERNAL.md, connected-account paths, the memory
// protocol with live horizons, and the skills catalog.
func platformPrompt(ctx context.Context, root, cwd, workspace, memoryRoot, skillsPrompt string, connections []connections.AgentConnection, agentNames []string, surface visualize.Surface, now time.Time) (string, error) {
	// These prompt files always render — an empty section tells every agent the
	// file exists and is editable, instead of silently vanishing.
	agents, err := ReadPromptFile(root, "AGENTS.md")
	if err != nil {
		return "", err
	}
	soul, err := ReadPromptFile(root, "SOUL.md")
	if err != nil {
		return "", err
	}
	internal, err := ReadPromptFile(root, "INTERNAL.md")
	if err != nil {
		return "", err
	}
	memory, err := memoryData(memoryRoot, now)
	if err != nil {
		return "", err
	}
	return jazplatform.Render(jazplatform.Data{
		Agents:          orEmpty(agents),
		AgentNames:      acp.SelectableAgentNames(agentNames),
		Date:            now.Format("January 2, 2006"),
		Time:            now.Format("15:04:05 MST"),
		Timezone:        now.Format("MST (UTCZ07:00)"),
		Weekday:         now.Format("Monday"),
		Human:           now.Format("Monday, January 2, 2006 at 15:04:05 MST"),
		Cwd:             strings.TrimSpace(cwd),
		Device:          sessioncontext.ClientPlatform(ctx),
		RuntimePaths:    runtimePaths(root, workspace),
		Soul:            orEmpty(soul),
		Internal:        orEmpty(internal),
		ArtifactSurface: string(visualize.NormalizeSurface(string(surface))),
		Memory:          memory,
		Connections:     connections,
		Skills:          strings.TrimSpace(skillsPrompt),
	})
}

func runtimePaths(root, workspace string) jazplatform.RuntimePaths {
	root = displayRoot(root)
	defaultWorkspacePath := strings.TrimSpace(workspace)
	if defaultWorkspacePath == "" {
		defaultWorkspacePath = defaultWorkspace(root)
	}
	return jazplatform.RuntimePaths{
		Root:             root,
		AgentsPath:       filepath.Join(root, "AGENTS.md"),
		SoulPath:         filepath.Join(root, "SOUL.md"),
		InternalPath:     filepath.Join(root, "INTERNAL.md"),
		SkillsPath:       filepath.Join(root, "skills"),
		SessionsPath:     filepath.Join(root, "sessions"),
		DefaultWorkspace: defaultWorkspacePath,
		WorktreesPath:    filepath.Join(defaultWorkspacePath, ".worktrees"),
	}
}

func displayRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return "~/.jaz"
	}
	return root
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
