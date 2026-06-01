package acp

type Config struct {
	Agents map[string]AgentConfig
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
