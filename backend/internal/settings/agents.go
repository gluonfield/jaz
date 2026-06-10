package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	AgentSettingsNamespace = "agents"
	AgentDefaultsKey       = "defaults"
	legacyCodexACPCommand  = `npx -y @zed-industries/codex-acp -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'`
	legacyGrokACPCommand   = `grok --no-auto-update agent --no-leader stdio`
	legacyClaudeCodeModel  = "claude-sonnet-4-5"
)

// Previous built-in claude commands; stored settings still matching one are
// auto-upgraded to the current default on merge.
var legacyClaudeCodeCommands = []string{
	"npx -y @agentclientprotocol/claude-agent-acp@0.39.0",
	"npx -y @agentclientprotocol/claude-agent-acp@0.43.0",
}

type NativeAgentDefaults struct {
	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type ACPAgentDefaults struct {
	Enabled         bool     `json:"enabled"`
	Command         string   `json:"command,omitempty"`
	LegacyArgs      []string `json:"args,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
}

type AgentDefaults struct {
	Native NativeAgentDefaults         `json:"native"`
	ACP    map[string]ACPAgentDefaults `json:"acp"`
}

func DefaultAgentDefaults() AgentDefaults {
	return AgentDefaults{
		Native: defaultNativeAgentDefaults(),
		ACP:    map[string]ACPAgentDefaults{},
	}
}

func defaultNativeAgentDefaults() NativeAgentDefaults {
	providerID := provider.ProviderOpenRouter
	meta, _ := provider.NativeProviderByID(providerID)
	return NativeAgentDefaults{
		ModelProvider:   providerID,
		Model:           strings.TrimSpace(meta.DefaultModel),
		ReasoningEffort: strings.TrimSpace(meta.DefaultReasoningEffort),
	}
}

func AgentDefaultsFromCatalog(catalog acp.AgentCatalog) AgentDefaults {
	seed := DefaultAgentDefaults()
	seed.ACP = map[string]ACPAgentDefaults{}
	for _, name := range catalog.Names() {
		agent, _ := catalog.Agent(name)
		command := CommandLine(agent.Command, agent.Args)
		seed.ACP[name] = ACPAgentDefaults{
			Enabled:         strings.TrimSpace(command) != "" || strings.TrimSpace(agent.URL) != "",
			Command:         command,
			Model:           strings.TrimSpace(agent.Model),
			ReasoningEffort: strings.TrimSpace(agent.ReasoningEffort),
		}
	}
	return seed
}

func LoadAgentDefaults(store storage.SettingsStorage) (AgentDefaults, error) {
	setting, err := store.LoadSetting(AgentSettingsNamespace, AgentDefaultsKey)
	if err != nil {
		return AgentDefaults{}, err
	}
	var defaults AgentDefaults
	if err := json.Unmarshal(setting.Value, &defaults); err != nil {
		return AgentDefaults{}, err
	}
	if defaults.ACP == nil {
		defaults.ACP = map[string]ACPAgentDefaults{}
	}
	defaults.ACP = canonicalizeACPDefaults(defaults.ACP)
	return defaults, nil
}

func SaveAgentDefaults(store storage.SettingsStorage, defaults AgentDefaults) (AgentDefaults, error) {
	defaults.ACP = canonicalizeACPDefaults(defaults.ACP)
	data, err := json.Marshal(defaults)
	if err != nil {
		return AgentDefaults{}, err
	}
	if _, err := store.SaveSetting(AgentSettingsNamespace, AgentDefaultsKey, data); err != nil {
		return AgentDefaults{}, err
	}
	return LoadAgentDefaults(store)
}

func EnsureAgentDefaults(store storage.SettingsStorage, seed AgentDefaults) error {
	current, err := LoadAgentDefaults(store)
	if err == nil {
		merged := MergeAgentDefaults(current, seed, agentNames(seed))
		if agentDefaultsEqual(current, merged) && storedAgentDefaultsEqual(store, current) {
			return nil
		}
		_, err := SaveAgentDefaults(store, merged)
		return err
	} else if !errors.Is(err, storage.ErrSettingNotFound) {
		return err
	}
	_, err = SaveAgentDefaults(store, seed)
	return err
}

func storedAgentDefaultsEqual(store storage.SettingsStorage, defaults AgentDefaults) bool {
	setting, err := store.LoadSetting(AgentSettingsNamespace, AgentDefaultsKey)
	if err != nil {
		return false
	}
	var stored AgentDefaults
	if err := json.Unmarshal(setting.Value, &stored); err != nil {
		return false
	}
	if stored.ACP == nil {
		stored.ACP = map[string]ACPAgentDefaults{}
	}
	return agentDefaultsEqual(stored, defaults)
}

func agentDefaultsEqual(a, b AgentDefaults) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func agentNames(defaults AgentDefaults) []string {
	names := make([]string, 0, len(defaults.ACP))
	for name := range defaults.ACP {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func NormalizeAgentDefaults(input AgentDefaults, catalog acp.AgentCatalog) (AgentDefaults, error) {
	agentNames := catalog.Names()
	allowed := map[string]struct{}{}
	for _, name := range agentNames {
		name = acp.CanonicalAgentName(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	native, err := NormalizeNativeDefaults(input.Native)
	if err != nil {
		return AgentDefaults{}, err
	}

	next := AgentDefaults{
		Native: native,
		ACP:    map[string]ACPAgentDefaults{},
	}
	inputACP := canonicalizeACPDefaults(input.ACP)
	for name := range inputACP {
		if _, ok := allowed[name]; !ok {
			return AgentDefaults{}, fmt.Errorf("unknown acp agent %q", name)
		}
	}
	for _, name := range sortedAgentNames(agentNames) {
		name = acp.CanonicalAgentName(name)
		base, _ := catalog.Agent(name)
		current := inputACP[name]
		effort, err := provider.NormalizeReasoningEffort(current.ReasoningEffort)
		if err != nil {
			return AgentDefaults{}, err
		}
		command := strings.TrimSpace(current.Command)
		if current.Enabled && command == "" && strings.TrimSpace(base.URL) == "" {
			return AgentDefaults{}, fmt.Errorf("acp agent %q command is required when enabled", name)
		}
		if current.Enabled && command != "" {
			if executable, _, err := ParseCommandLine(command); err != nil {
				return AgentDefaults{}, fmt.Errorf("acp agent %q command: %w", name, err)
			} else if executable == "" {
				return AgentDefaults{}, fmt.Errorf("acp agent %q command is required when enabled", name)
			}
		}
		next.ACP[name] = ACPAgentDefaults{
			Enabled:         current.Enabled,
			Command:         command,
			Model:           strings.TrimSpace(current.Model),
			ReasoningEffort: effort,
		}
	}
	return next, nil
}

func NormalizeNativeDefaults(input NativeAgentDefaults) (NativeAgentDefaults, error) {
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	if input.ModelProvider == "" {
		return NativeAgentDefaults{}, fmt.Errorf("native provider is required")
	}
	modelProvider, err := provider.NormalizeNativeProviderID(input.ModelProvider)
	if err != nil {
		return NativeAgentDefaults{}, err
	}
	input.ModelProvider = modelProvider
	input.Model = strings.TrimSpace(input.Model)
	if input.Model == "" {
		return NativeAgentDefaults{}, fmt.Errorf("native model is required")
	}
	effort, err := provider.NormalizeReasoningEffort(input.ReasoningEffort)
	if err != nil {
		return NativeAgentDefaults{}, err
	}
	input.ReasoningEffort = effort
	return input, nil
}

func MergeAgentDefaults(stored, seed AgentDefaults, agentNames []string) AgentDefaults {
	if stored.ACP == nil {
		stored.ACP = map[string]ACPAgentDefaults{}
	}
	stored.ACP = canonicalizeACPDefaults(stored.ACP)
	if strings.TrimSpace(stored.Native.ModelProvider) == "" {
		stored.Native.ModelProvider = seed.Native.ModelProvider
	}
	if strings.TrimSpace(stored.Native.Model) == "" {
		stored.Native.Model = seed.Native.Model
	}
	next := AgentDefaults{
		Native: stored.Native,
		ACP:    map[string]ACPAgentDefaults{},
	}
	for _, name := range sortedAgentNames(agentNames) {
		name = acp.CanonicalAgentName(name)
		value, ok := stored.ACP[name]
		if !ok {
			value = seed.ACP[name]
		} else {
			value = mergeACPAgentDefaults(name, value, seed.ACP[name])
		}
		next.ACP[name] = value
	}
	return next
}

func mergeACPAgentDefaults(name string, stored, seed ACPAgentDefaults) ACPAgentDefaults {
	stored = collapseLegacyACPCommand(stored)
	switch {
	case strings.TrimSpace(stored.Command) == "":
		stored.Command = seed.Command
	case name == acp.AgentCodex && strings.TrimSpace(stored.Command) == legacyCodexACPCommand:
		stored.Command = seed.Command
	case name == acp.AgentClaude && isLegacyClaudeCodeCommand(stored.Command):
		stored.Command = seed.Command
	case name == acp.AgentGrok && strings.TrimSpace(stored.Command) == legacyGrokACPCommand:
		stored.Command = seed.Command
	}
	if name == acp.AgentClaude && strings.TrimSpace(stored.Model) == legacyClaudeCodeModel {
		stored.Model = seed.Model
	}
	return stored
}

func isLegacyClaudeCodeCommand(command string) bool {
	return slices.Contains(legacyClaudeCodeCommands, strings.TrimSpace(command))
}

func canonicalizeACPDefaults(in map[string]ACPAgentDefaults) map[string]ACPAgentDefaults {
	out := map[string]ACPAgentDefaults{}
	for name, agent := range in {
		canonical := acp.CanonicalAgentName(name)
		if canonical == "" || canonical != strings.TrimSpace(name) {
			continue
		}
		out[canonical] = collapseLegacyACPCommand(agent)
	}
	for name, agent := range in {
		canonical := acp.CanonicalAgentName(name)
		if canonical == "" || canonical == strings.TrimSpace(name) {
			continue
		}
		if _, ok := out[canonical]; !ok {
			out[canonical] = collapseLegacyACPCommand(agent)
		}
	}
	return out
}

type ACPConfigSource struct {
	store   storage.SettingsStorage
	catalog acp.AgentCatalog
}

func NewACPConfigSource(store storage.SettingsStorage, catalog acp.AgentCatalog) *ACPConfigSource {
	return &ACPConfigSource{store: store, catalog: catalog}
}

func (s *ACPConfigSource) AgentConfig(name string) (acp.AgentConfig, bool, error) {
	name = acp.CanonicalAgentName(name)
	cfg, ok := s.catalog.Agent(name)
	if !ok {
		return acp.AgentConfig{}, false, nil
	}
	defaults, err := LoadAgentDefaults(s.store)
	if err != nil {
		return acp.AgentConfig{}, false, err
	}
	agent, ok := defaults.ACP[name]
	if !ok || !agent.Enabled {
		return acp.AgentConfig{}, false, nil
	}
	command := strings.TrimSpace(agent.Command)
	if command == "" && strings.TrimSpace(cfg.URL) == "" {
		return acp.AgentConfig{}, false, fmt.Errorf("acp agent %q command is required when enabled", name)
	}
	if command != "" {
		executable, args, err := ParseCommandLine(command)
		if err != nil {
			return acp.AgentConfig{}, false, fmt.Errorf("acp agent %q command: %w", name, err)
		}
		if executable == "" {
			return acp.AgentConfig{}, false, fmt.Errorf("acp agent %q command is required when enabled", name)
		}
		cfg.Command, cfg.Args = executable, args
	}
	cfg.Model = strings.TrimSpace(agent.Model)
	cfg.ReasoningEffort = strings.TrimSpace(agent.ReasoningEffort)
	return cfg, true, nil
}

func (s *ACPConfigSource) EnabledAgentNames() ([]string, error) {
	defaults, err := LoadAgentDefaults(s.store)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, name := range s.catalog.Names() {
		agent, ok := defaults.ACP[name]
		if !ok || !agent.Enabled {
			continue
		}
		base, _ := s.catalog.Agent(name)
		if strings.TrimSpace(agent.Command) == "" && strings.TrimSpace(base.URL) == "" {
			return nil, fmt.Errorf("acp agent %q command is required when enabled", name)
		}
		names = append(names, name)
	}
	return names, nil
}

func collapseLegacyACPCommand(agent ACPAgentDefaults) ACPAgentDefaults {
	if len(agent.LegacyArgs) > 0 {
		agent.Command = CommandLine(agent.Command, agent.LegacyArgs)
		agent.LegacyArgs = nil
	}
	return agent
}

func sortedAgentNames(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}

func CommandLine(command string, args []string) string {
	parts := []string{}
	if command = strings.TrimSpace(command); command != "" {
		parts = append(parts, shellQuote(command))
	}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			parts = append(parts, shellQuote(arg))
		}
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\"'\\$`!|&;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
