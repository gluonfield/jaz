package acp

import (
	"context"
	"runtime"
	"sort"
	"strings"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	AgentJaz      = "jaz"
	AgentCodex    = "codex"
	AgentClaude   = "claude"
	AgentGrok     = "grok"
	AgentOpenCode = "opencode"

	AgentProviderModeNone          = ""
	AgentProviderModeAgentDefaults = "agent_defaults"

	AuthModeAuto        = "auto"
	AuthModeExistingCLI = "existing_cli"
	AuthModeJazProfile  = "jaz_profile"

	codexACPPackage = "@jazchat/codex-acp@0.16.7"
)

func CanonicalAgentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.ReplaceAll(name, "_", "-") == "grok-build" {
		return AgentGrok
	}
	return name
}

// SystemPromptSource supplies the full ACP session extension (AGENTS.md,
// memory, skills) injected at session creation.
type SystemPromptSource interface {
	ACPPromptForContext(ctx context.Context, cwd, surface string) (string, error)
}

type SessionPromptExtensionResolver func(storage.Session) (promptmodule.Modules, error)

func promptWithModules(base string, modules promptmodule.Modules) string {
	return promptmodule.New(base).Append(modules...).Text()
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
	Agents      map[string]AgentConfig
	AgentSource AgentConfigSource
	Root        string
	Workspace   string
	Env         map[string]string
	// Providers is a static snapshot used by the read-time auth/readiness probes
	// (and tests). ProviderSource, when set, is the live merged registry the
	// manager reads per spawn so runtime provider changes reach new agents.
	Providers      map[string]provider.ModelProviderConfig
	ProviderSource provider.Source
	SystemPrompt   SystemPromptSource
	MCPStore       mcpconfig.ServerReader
	ResumePrompt   SessionPromptExtensionResolver
}

type AgentConfig struct {
	Command                 string
	Args                    []string
	Local                   bool
	ProviderMode            string
	ModelProviderCapability string
	ModelProvider           string
	Model                   string
	ReasoningEffort         string
	URL                     string
	Token                   string
	Auth                    AgentAuthConfig
	Env                     map[string]string
	Cwd                     string
}

func (c AgentConfig) RequiresCommand() bool {
	return !c.Local && strings.TrimSpace(c.URL) == ""
}

func (c AgentConfig) SupportsAuth() bool {
	return !c.Local
}

func (c AgentConfig) UsesModelProvider() bool {
	return strings.TrimSpace(c.ProviderMode) == AgentProviderModeAgentDefaults
}

func (c AgentConfig) UsesProvider() bool {
	return c.UsesModelProvider()
}

func (c AgentConfig) ProviderQualifiedModel() string {
	modelProvider := strings.TrimSpace(c.ModelProvider)
	model := strings.TrimSpace(c.Model)
	if !c.UsesModelProvider() || modelProvider == "" || model == "" {
		return model
	}
	if embedded, _ := provider.SplitProviderModel(model); embedded == modelProvider {
		return model
	}
	return modelProvider + "/" + model
}

func (c AgentConfig) NormalizeProviderModel(defaultProvider string) AgentConfig {
	if !c.UsesModelProvider() {
		return c
	}
	modelProvider := strings.TrimSpace(c.ModelProvider)
	model := strings.TrimSpace(c.Model)
	if embedded, rest := provider.SplitProviderModel(model); embedded != "" {
		switch {
		case modelProvider == "":
			modelProvider = embedded
			model = rest
		case embedded == modelProvider:
			model = rest
		}
	}
	if modelProvider == "" {
		modelProvider = strings.TrimSpace(defaultProvider)
	}
	c.ModelProvider = modelProvider
	c.Model = model
	return c
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
		AgentCodex: codexBuiltinAgent(runtime.GOOS),
		AgentClaude: {
			Command:         "npx",
			Args:            []string{"-y", "@agentclientprotocol/claude-agent-acp@0.44.0"},
			Model:           "default",
			ReasoningEffort: "xhigh",
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
			Command:                 "npx",
			Args:                    []string{"-y", "opencode-ai@1.17.7", "acp"},
			ProviderMode:            AgentProviderModeAgentDefaults,
			ModelProviderCapability: provider.CapabilityOpenCode,
			ModelProvider:           provider.ProviderOpenRouter,
			Model:                   "openai/gpt-5.4-mini",
			ReasoningEffort:         "",
		},
	}
}

func codexBuiltinAgent(goos string) AgentConfig {
	command := "npx"
	if goos == "windows" {
		command = "npx.cmd"
	}
	return AgentConfig{
		Command: command,
		Args: []string{
			"-y", codexACPPackage,
			"-c", `sandbox_mode="danger-full-access"`,
			"-c", `approval_policy="never"`,
			"-c", `features.tool_search_always_defer_mcp_tools=true`,
			"-c", `suppress_unstable_features_warning=true`,
		},
		Model:           "gpt-5.5",
		ReasoningEffort: "xhigh",
	}
}

func MergeAgents(base, override map[string]AgentConfig) AgentCatalog {
	out := AgentCatalog{}
	for name, cfg := range base {
		out[CanonicalAgentName(name)] = cfg
	}
	for name, cfg := range override {
		name = CanonicalAgentName(name)
		if current, ok := out[name]; ok {
			out[name] = mergeAgentConfig(current, cfg)
			continue
		}
		out[name] = cfg
	}
	return out
}

func mergeAgentConfig(base, override AgentConfig) AgentConfig {
	next := base
	if strings.TrimSpace(override.Command) != "" {
		next.Command = override.Command
		next.Args = append([]string(nil), override.Args...)
	} else if override.Args != nil {
		next.Args = append([]string(nil), override.Args...)
	}
	if override.Local {
		next.Local = true
	}
	if strings.TrimSpace(override.ProviderMode) != "" {
		next.ProviderMode = override.ProviderMode
	}
	if strings.TrimSpace(override.ModelProviderCapability) != "" {
		next.ModelProviderCapability = override.ModelProviderCapability
	}
	if strings.TrimSpace(override.ModelProvider) != "" {
		next.ModelProvider = override.ModelProvider
	}
	if strings.TrimSpace(override.Model) != "" {
		next.Model = override.Model
	}
	if strings.TrimSpace(override.ReasoningEffort) != "" {
		next.ReasoningEffort = override.ReasoningEffort
	}
	if strings.TrimSpace(override.URL) != "" {
		next.URL = override.URL
	}
	if strings.TrimSpace(override.Token) != "" {
		next.Token = override.Token
	}
	next.Auth = mergeAgentAuthConfig(next.Auth, override.Auth)
	if override.Env != nil {
		next.Env = mergeStringMap(next.Env, override.Env)
	}
	if strings.TrimSpace(override.Cwd) != "" {
		next.Cwd = override.Cwd
	}
	return next
}

func mergeAgentAuthConfig(base, override AgentAuthConfig) AgentAuthConfig {
	if strings.TrimSpace(override.Mode) != "" {
		base.Mode = override.Mode
	}
	if strings.TrimSpace(override.Path) != "" {
		base.Path = override.Path
	}
	return base
}

func mergeStringMap(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}
