package acp

import (
	"sort"
	"strings"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/provider"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	AgentCodex    = "codex"
	AgentClaude   = "claude"
	AgentGrok     = "grok"
	AgentOpenCode = "opencode"

	AuthModeAuto        = "auto"
	AuthModeExistingCLI = "existing_cli"
	AuthModeJazProfile  = "jaz_profile"
)

func CanonicalAgentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.ReplaceAll(name, "_", "-") == "claude-code" {
		return AgentClaude
	}
	if strings.ReplaceAll(name, "_", "-") == "grok-build" {
		return AgentGrok
	}
	return name
}

// SystemPromptSource supplies prompts for ACP sessions: ACPPrompt is the full
// session extension (AGENTS.md, memory, skills) injected at session creation,
// SkillsPrompt is the skills catalog alone for per-turn $skill resolution.
// Both are consulted on use, not at startup, so prompt and skill edits reach
// new sessions and turns without a restart.
type SystemPromptSource interface {
	ACPPrompt() (string, error)
	SkillsPrompt() (string, error)
}

// systemPromptMeta wraps prompt in the session _meta payload understood by
// the named agent. ACP has no standard system-prompt field, so each adapter
// defines its own extension key; every form below appends to the agent's own
// system prompt rather than replacing it:
//   - claude-agent-acp reads _meta.systemPrompt; {"append": ...} extends the
//     Claude Code preset, while a bare string would replace it.
//   - grok reads _meta.rules and ignores _meta.systemPrompt.
//   - codex-acp (Jaz fork) appends a _meta.systemPrompt string as developer
//     instructions; upstream codex-acp ignores _meta entirely.
//
// Unknown agents get the codex-style bare string.
func systemPromptMeta(agent, prompt string) map[string]any {
	switch CanonicalAgentName(agent) {
	case AgentClaude:
		return map[string]any{"systemPrompt": map[string]any{"append": prompt}}
	case AgentGrok:
		return map[string]any{"rules": prompt}
	default:
		return map[string]any{"systemPrompt": prompt}
	}
}

type Config struct {
	Agents       map[string]AgentConfig
	AgentSource  AgentConfigSource
	Root         string
	Workspace    string
	Env          map[string]string
	Providers    map[string]provider.ModelProviderConfig
	SystemPrompt SystemPromptSource
	MCPStore     mcpconfig.ServerReader
	MCPTokens    integrationoauth.Store
}

type AgentConfig struct {
	Command         string
	Args            []string
	Model           string
	ReasoningEffort string
	URL             string
	Token           string
	Auth            AgentAuthConfig
	Env             map[string]string
	Cwd             string
}

type AgentAuthConfig struct {
	Mode string `json:"mode,omitempty"`
	Path string `json:"path,omitempty"`
}

type AgentCatalog map[string]AgentConfig

type AgentConfigSource interface {
	AgentConfig(name string) (AgentConfig, bool, error)
	EnabledAgentNames() ([]string, error)
}

func (c AgentCatalog) Agent(name string) (AgentConfig, bool) {
	if c == nil {
		return AgentConfig{}, false
	}
	name = CanonicalAgentName(name)
	agent, ok := c[name]
	return agent, ok
}

func (c AgentCatalog) AgentConfig(name string) (AgentConfig, bool, error) {
	agent, ok := c.Agent(name)
	return agent, ok, nil
}

func (c AgentCatalog) Names() []string {
	names := make([]string, 0, len(c))
	for name := range c {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c AgentCatalog) EnabledAgentNames() ([]string, error) {
	return c.Names(), nil
}

func BuiltinAgents() AgentCatalog {
	return AgentCatalog{
		AgentCodex: {
			Command: "npx",
			Args: []string{
				"-y", "@jazchat/codex-acp@0.16.1",
				"-c", `sandbox_mode="danger-full-access"`,
				"-c", `approval_policy="never"`,
			},
			Model:           "gpt-5.5",
			ReasoningEffort: "medium",
		},
		AgentClaude: {
			Command:         "npx",
			Args:            []string{"-y", "@agentclientprotocol/claude-agent-acp@0.44.0"},
			Model:           "default",
			ReasoningEffort: "medium",
		},
		AgentGrok: {
			Command: "grok",
			Args: []string{
				"--no-auto-update",
				"agent",
				"--no-leader",
				"--always-approve",
				"stdio",
			},
			Model:           "grok-build",
			ReasoningEffort: "medium",
		},
		AgentOpenCode: {
			Command:         "npx",
			Args:            []string{"-y", "opencode-ai@1.17.7", "acp"},
			Model:           "openrouter/openai/gpt-5.4-mini",
			ReasoningEffort: "",
		},
	}
}

func MergeAgents(base, override map[string]AgentConfig) AgentCatalog {
	out := AgentCatalog{}
	for name, cfg := range base {
		out[CanonicalAgentName(name)] = cfg
	}
	for name, cfg := range override {
		out[CanonicalAgentName(name)] = cfg
	}
	return out
}
