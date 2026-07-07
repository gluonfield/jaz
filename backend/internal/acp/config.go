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
	AgentJaz         = "jaz"
	AgentCodex       = "codex"
	AgentClaude      = "claude"
	AgentGrok        = "grok"
	AgentOpenCode    = "opencode"
	AgentAntigravity = "antigravity"

	AgentProviderModeNone          = ""
	AgentProviderModeAgentDefaults = "agent_defaults"

	AuthModeAuto        = "auto"
	AuthModeExistingCLI = "existing_cli"
	AuthModeJazProfile  = "jaz_profile"
)

func CanonicalAgentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.ReplaceAll(name, "_", "-") == "grok-build" {
		return AgentGrok
	}
	return name
}

func AgentSupportsCompact(name string) bool {
	switch CanonicalAgentName(name) {
	case AgentCodex, AgentClaude:
		return true
	default:
		return false
	}
}

// SystemPromptSource supplies the full ACP session extension injected at
// session creation: platform context, prompt files, connections, memory, and
// skills.
type SystemPromptSource interface {
	ACPPromptForContext(ctx context.Context, cwd, surface string) (string, error)
}

type PromptModuleOptions struct {
	Connections bool
}

type SystemPromptModules interface {
	PromptModulesForContext(context.Context, PromptModuleOptions) (promptmodule.Modules, error)
}

type SessionPromptExtensionResolver func(storage.Session) (promptmodule.Modules, error)

type ReasoningEffortValidator interface {
	ValidateReasoningEffort(agent, providerID, model, effort string) error
}

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
	Adapters    AdapterResolver
	Root        string
	Workspace   string
	Env         map[string]string
	// Providers is a static snapshot used by the read-time auth/readiness probes
	// (and tests). ProviderSource, when set, is the live merged registry the
	// manager reads per spawn so runtime provider changes reach new agents.
	Providers      map[string]provider.ModelProviderConfig
	ProviderSource provider.Source
	ModelCatalog   ReasoningEffortValidator
	SystemPrompt   SystemPromptSource
	MCPStore       mcpconfig.ServerReader
	ResumePrompt   SessionPromptExtensionResolver
}

type AgentConfig struct {
	Command                 string
	Args                    []string
	ManagedAdapter          string
	ManagedAdapterArgs      []string
	ManagedTool             string
	ManagedToolAdapterArg   string
	Local                   bool
	ProviderMode            string
	ModelProviderCapability string
	ModelProvider           string
	AuthProviderID          string
	Model                   string
	ReasoningEffort         string
	URL                     string
	Token                   string
	Auth                    AgentAuthConfig
	Env                     map[string]string
	Cwd                     string
	// AdapterBinDir is the managed-adapter bundle dir. LoginBinDir is searched
	// before PATH for companion CLIs. Runtime-only, never persisted.
	AdapterBinDir string
	LoginBinDir   string
}

func (c AgentConfig) RequiresCommand() bool {
	return !c.Local && strings.TrimSpace(c.URL) == "" && strings.TrimSpace(c.ManagedAdapter) == ""
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

func (c AgentConfig) ProviderNativeModel() string {
	return strings.TrimSpace(c.Model)
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

type AdapterLaunch struct {
	Command string
	Args    []string
	Env     map[string]string
}

type AdapterResolver interface {
	ResolveAdapter(ctx context.Context, name string) (AdapterLaunch, error)
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

func SelectableAgentNames(names []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = CanonicalAgentName(name)
		if name == "" || name == AgentJaz {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func SelectableAgentCatalog(catalog AgentCatalog) AgentCatalog {
	out := AgentCatalog{}
	for _, name := range SelectableAgentNames(catalog.Names()) {
		if cfg, ok := catalog.Agent(name); ok {
			out[name] = cfg
		}
	}
	return out
}

func BuiltinAgents() AgentCatalog {
	return AgentCatalog{
		AgentCodex: codexBuiltinAgent(runtime.GOOS),
		AgentClaude: {
			ManagedAdapter:  "claude",
			Model:           "default",
			ReasoningEffort: DefaultAgentReasoningEffort(AgentClaude),
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
			ReasoningEffort: DefaultAgentReasoningEffort(AgentGrok),
		},
		AgentOpenCode: {
			Command:                 "npx",
			Args:                    []string{"-y", "opencode-ai@1.17.7", "acp"},
			ProviderMode:            AgentProviderModeAgentDefaults,
			ModelProviderCapability: provider.CapabilityOpenCode,
			ModelProvider:           provider.ProviderOpenRouter,
			Model:                   "openai/gpt-5.4-mini",
			ReasoningEffort:         DefaultAgentReasoningEffort(AgentOpenCode),
		},
		AgentAntigravity: {
			ManagedAdapter:        "antigravity",
			ManagedAdapterArgs:    []string{"--auth=auto", "--dangerously-skip-permissions"},
			ManagedTool:           "antigravity-cli",
			ManagedToolAdapterArg: "--agy",
		},
	}
}

func codexBuiltinAgent(_ string) AgentConfig {
	return AgentConfig{
		ManagedAdapter: "codex",
		ManagedAdapterArgs: []string{
			"-c", `sandbox_mode="danger-full-access"`,
			"-c", `approval_policy="never"`,
			"-c", `features.tool_search_always_defer_mcp_tools=true`,
			"-c", `suppress_unstable_features_warning=true`,
		},
		ProviderMode:            AgentProviderModeAgentDefaults,
		ModelProviderCapability: provider.CapabilityCodex,
		ModelProvider:           provider.ProviderOpenAI,
		AuthProviderID:          provider.ProviderOpenAI,
		Model:                   "gpt-5.5",
		ReasoningEffort:         DefaultAgentReasoningEffort(AgentCodex),
	}
}

func MergeAgents(base, override map[string]AgentConfig) AgentCatalog {
	out := AgentCatalog{}
	for name, cfg := range base {
		out[CanonicalAgentName(name)] = canonicalAgentConfig(cfg)
	}
	for name, cfg := range override {
		name = CanonicalAgentName(name)
		if current, ok := out[name]; ok {
			out[name] = mergeAgentConfig(current, cfg)
			continue
		}
		out[name] = canonicalAgentConfig(cfg)
	}
	return out
}

func canonicalAgentConfig(cfg AgentConfig) AgentConfig {
	url := strings.TrimSpace(cfg.URL)
	adapter := strings.TrimSpace(cfg.ManagedAdapter)
	command := strings.TrimSpace(cfg.Command)
	switch {
	case cfg.Local:
		cfg.useLocalLaunch()
	case url != "":
		cfg.useURLLaunch(url, strings.TrimSpace(cfg.Token))
	case adapter != "":
		cfg.useManagedAdapterLaunch(adapter, cfg.ManagedAdapterArgs)
	case command != "":
		cfg.useCommandLaunch(command, cfg.Args)
	}
	cfg.ManagedTool = strings.TrimSpace(cfg.ManagedTool)
	cfg.ManagedToolAdapterArg = strings.TrimSpace(cfg.ManagedToolAdapterArg)
	return cfg
}

func mergeAgentConfig(base, override AgentConfig) AgentConfig {
	next := base
	url := strings.TrimSpace(override.URL)
	adapter := strings.TrimSpace(override.ManagedAdapter)
	command := strings.TrimSpace(override.Command)
	switch {
	case override.Local:
		next.useLocalLaunch()
	case url != "":
		next.useURLLaunch(url, "")
	case adapter != "":
		next.useManagedAdapterLaunch(adapter, override.ManagedAdapterArgs)
	case command != "":
		next.useCommandLaunch(command, override.Args)
	case override.ManagedAdapterArgs != nil && strings.TrimSpace(next.ManagedAdapter) != "":
		next.ManagedAdapterArgs = append([]string(nil), override.ManagedAdapterArgs...)
	case override.Args != nil && next.RequiresCommand():
		next.Args = append([]string(nil), override.Args...)
	}
	if tool := strings.TrimSpace(override.ManagedTool); tool != "" {
		next.ManagedTool = tool
	}
	if arg := strings.TrimSpace(override.ManagedToolAdapterArg); arg != "" {
		next.ManagedToolAdapterArg = arg
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
	if token := strings.TrimSpace(override.Token); token != "" && strings.TrimSpace(next.URL) != "" {
		next.Token = token
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

func (c *AgentConfig) useCommandLaunch(command string, args []string) {
	c.Command = command
	c.Args = append([]string(nil), args...)
	c.ManagedAdapter = ""
	c.ManagedAdapterArgs = nil
	c.Local = false
	c.URL = ""
	c.Token = ""
}

func (c *AgentConfig) useManagedAdapterLaunch(adapter string, args []string) {
	c.Command = ""
	c.Args = nil
	c.ManagedAdapter = adapter
	c.ManagedAdapterArgs = append([]string(nil), args...)
	c.Local = false
	c.URL = ""
	c.Token = ""
}

func (c *AgentConfig) useURLLaunch(url, token string) {
	c.Command = ""
	c.Args = nil
	c.ManagedAdapter = ""
	c.ManagedAdapterArgs = nil
	c.Local = false
	c.URL = url
	c.Token = token
}

func (c *AgentConfig) useLocalLaunch() {
	c.Command = ""
	c.Args = nil
	c.ManagedAdapter = ""
	c.ManagedAdapterArgs = nil
	c.Local = true
	c.URL = ""
	c.Token = ""
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
