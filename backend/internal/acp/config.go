package acp

import mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"

var fullAccessModes = []string{"full-access", "yolo"}

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
	MCPStore     mcpconfig.ServerReader
}

type AgentConfig struct {
	Command         string
	Args            []string
	Model           string
	ReasoningEffort string
	URL             string
	Token           string
	Env             map[string]string
	Cwd             string
}

func (c Config) Agent(name string) (AgentConfig, bool) {
	if c.Agents == nil {
		return AgentConfig{}, false
	}
	agent, ok := c.Agents[name]
	return agent, ok
}
