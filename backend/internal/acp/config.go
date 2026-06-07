package acp

// Mode IDs that grant unattended full access, in preference order. Agents
// name the same capability differently: Codex exposes "full-access", the
// Claude Code adapter "bypassPermissions", Gemini CLI "yolo".
var fullAccessModes = []string{"full-access", "bypassPermissions", "yolo"}

// SystemPromptSource supplies the system prompt for new ACP sessions. It is
// consulted at session creation, not at startup, so skill edits reach new
// sessions without a restart.
type SystemPromptSource interface {
	SkillsPrompt() string
}

type Config struct {
	Agents       map[string]AgentConfig
	Root         string
	Workspace    string
	Env          map[string]string
	SystemPrompt SystemPromptSource
}

type AgentConfig struct {
	Command string
	Args    []string
	URL     string
	Token   string
	Env     map[string]string
	Cwd     string
}

func (c Config) Agent(name string) (AgentConfig, bool) {
	if c.Agents == nil {
		return AgentConfig{}, false
	}
	agent, ok := c.Agents[name]
	return agent, ok
}
