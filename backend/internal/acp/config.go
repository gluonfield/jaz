package acp

const fullAccessMode = "full-access"

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
