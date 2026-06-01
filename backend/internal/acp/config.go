package acp

const (
	CodexACPVersion      = "0.15.0"
	ClaudeCodeACPVersion = "0.39.0"
	fullAccessMode       = "full-access"
)

type Config struct {
	Agents    map[string]AgentConfig
	Root      string
	Workspace string
	Env       map[string]string
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
	base, hasDefault := defaultAgent(name)
	if c.Agents == nil {
		return base, hasDefault
	}
	agent, ok := c.Agents[name]
	if !ok {
		return base, hasDefault
	}
	if agent.Command == "" && hasDefault {
		agent.Command = base.Command
	}
	if len(agent.Args) == 0 && hasDefault {
		agent.Args = base.Args
	}
	return agent, true
}

func defaultAgent(name string) (AgentConfig, bool) {
	switch name {
	case "codex":
		return npxAgent("@zed-industries/codex-acp", CodexACPVersion), true
	case "claude_code":
		return npxAgent("@agentclientprotocol/claude-agent-acp", ClaudeCodeACPVersion), true
	default:
		return AgentConfig{}, false
	}
}

func npxAgent(pkg, version string) AgentConfig {
	args := []string{"-y", pkg + "@" + version}
	if pkg == "@zed-industries/codex-acp" {
		args = append(args, "-c", "sandbox_mode=\"danger-full-access\"", "-c", "approval_policy=\"never\"")
	}
	return AgentConfig{Command: "npx", Args: args}
}
