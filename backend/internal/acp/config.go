package acp

// Mode IDs that grant unattended full access, in preference order. Agents
// name the same capability differently: Codex exposes "full-access", the
// Claude Code adapter "bypassPermissions", Gemini CLI "yolo".
var fullAccessModes = []string{"full-access", "bypassPermissions", "yolo"}

type Config struct {
	Agents       map[string]AgentConfig
	Root         string
	Workspace    string
	Env          map[string]string
	SystemPrompt string
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
