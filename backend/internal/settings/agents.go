package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	AgentSettingsNamespace = "agents"
	AgentDefaultsKey       = "defaults"
)

type ACPAgentDefaults struct {
	Enabled         bool                `json:"enabled"`
	ModelProvider   string              `json:"model_provider,omitempty"`
	Model           string              `json:"model,omitempty"`
	ReasoningEffort string              `json:"reasoning_effort,omitempty"`
	Auth            acp.AgentAuthConfig `json:"auth,omitempty"`
}

type AgentDefaults struct {
	ACP map[string]ACPAgentDefaults `json:"acp"`
}

type ReasoningEffortValidator interface {
	ValidateReasoningEffort(agent, providerID, model, effort string) error
}

func DefaultAgentDefaults() AgentDefaults {
	return AgentDefaults{
		ACP: map[string]ACPAgentDefaults{},
	}
}

func AgentDefaultsFromCatalog(catalog acp.AgentCatalog) AgentDefaults {
	seed := DefaultAgentDefaults()
	seed.ACP = map[string]ACPAgentDefaults{}
	for _, name := range catalog.Names() {
		agent, _ := catalog.Agent(name)
		seed.ACP[name] = ACPAgentDefaults{
			Enabled:         defaultACPAgentEnabled(name, agent),
			ModelProvider:   strings.TrimSpace(agent.ModelProvider),
			Model:           strings.TrimSpace(agent.Model),
			ReasoningEffort: strings.TrimSpace(agent.ReasoningEffort),
			Auth:            acp.AgentAuthConfig{Mode: acp.AuthModeAuto},
		}
	}
	return seed
}

func defaultACPAgentEnabled(name string, agent acp.AgentConfig) bool {
	if agent.Local || strings.TrimSpace(agent.URL) != "" {
		return true
	}
	if _, ok := acp.AgentAPIKey(name); ok {
		return false
	}
	return CommandLine(agent.Command, agent.Args) != ""
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

func LoadEffectiveAgentDefaults(store storage.SettingsStorage, catalog acp.AgentCatalog) (AgentDefaults, error) {
	seed := AgentDefaultsFromCatalog(catalog)
	current, err := LoadAgentDefaults(store)
	if err != nil {
		if !errors.Is(err, storage.ErrSettingNotFound) {
			return AgentDefaults{}, err
		}
		if _, err := SaveAgentDefaults(store, seed); err != nil {
			return AgentDefaults{}, err
		}
		current = seed
	}
	return MergeAgentDefaults(current, seed, catalog.Names()), nil
}

func DefaultWorkerAgent(defaults AgentDefaults) string {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude, acp.AgentOpenCode, acp.AgentAntigravity} {
		if current, ok := defaults.ACP[agent]; ok && current.Enabled {
			return agent
		}
	}
	return ""
}

func WorkerAgentModel(agent string, defaults AgentDefaults) string {
	switch acp.CanonicalAgentName(agent) {
	case acp.AgentCodex:
		if strings.TrimSpace(defaults.ACP[acp.AgentCodex].ModelProvider) == provider.ProviderOpenRouter {
			return provider.DefaultOpenRouterModel
		}
		return provider.DefaultOpenAIModel
	case acp.AgentClaude:
		return "default"
	case acp.AgentGrok:
		return modelcatalog.DefaultGrokModel
	case acp.AgentOpenCode:
		switch strings.TrimSpace(defaults.ACP[acp.AgentOpenCode].ModelProvider) {
		case provider.ProviderOpenAI:
			return provider.DefaultOpenAIModel
		case "", provider.ProviderOpenRouter:
			return provider.DefaultOpenRouterModel
		default:
			return ""
		}
	default:
		return ""
	}
}

func WorkerAgentReasoningEffort(agent string, defaults AgentDefaults) string {
	switch acp.CanonicalAgentName(agent) {
	case acp.AgentOpenCode:
		switch strings.TrimSpace(defaults.ACP[acp.AgentOpenCode].ModelProvider) {
		case "", provider.ProviderOpenAI, provider.ProviderOpenRouter:
			return acp.DefaultAgentReasoningEffort(agent)
		default:
			return ""
		}
	default:
		return acp.DefaultAgentReasoningEffort(agent)
	}
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

func NormalizeAgentDefaults(input AgentDefaults, catalog acp.AgentCatalog, validators ...ReasoningEffortValidator) (AgentDefaults, error) {
	var validator ReasoningEffortValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	agentNames := catalog.Names()
	allowed := map[string]struct{}{}
	for _, name := range agentNames {
		name = acp.CanonicalAgentName(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	next := AgentDefaults{
		ACP: map[string]ACPAgentDefaults{},
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
		effort, err := acp.NormalizeAgentReasoningEffort(name, current.ReasoningEffort)
		if err != nil {
			return AgentDefaults{}, err
		}
		auth, err := acp.NormalizeAgentAuthConfig(name, current.Auth)
		if err != nil {
			return AgentDefaults{}, err
		}
		modelProvider := strings.TrimSpace(current.ModelProvider)
		model := strings.TrimSpace(current.Model)
		if base.UsesModelProvider() {
			cfg := acp.AgentConfig{
				ProviderMode:  acp.AgentProviderModeAgentDefaults,
				ModelProvider: modelProvider,
				Model:         model,
			}.NormalizeProviderModel(base.ModelProvider)
			modelProvider = cfg.ModelProvider
			model = cfg.Model
		}
		if validator != nil {
			if err := validator.ValidateReasoningEffort(name, modelProvider, model, effort); err != nil {
				return AgentDefaults{}, err
			}
		}
		next.ACP[name] = ACPAgentDefaults{
			Enabled:         current.Enabled,
			ModelProvider:   modelProvider,
			Model:           model,
			ReasoningEffort: effort,
			Auth:            auth,
		}
	}
	return next, nil
}

func MergeAgentDefaults(stored, seed AgentDefaults, agentNames []string) AgentDefaults {
	if stored.ACP == nil {
		stored.ACP = map[string]ACPAgentDefaults{}
	}
	stored.ACP = canonicalizeACPDefaults(stored.ACP)
	next := AgentDefaults{
		ACP: map[string]ACPAgentDefaults{},
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
	if auth, err := acp.NormalizeAgentAuthConfig(name, stored.Auth); err == nil {
		stored.Auth = auth
	} else {
		stored.Auth = seed.Auth
	}
	if name == acp.AgentGrok && strings.TrimSpace(stored.Model) == "grok-build" {
		stored.Model = seed.Model
	}
	if strings.TrimSpace(seed.ModelProvider) != "" {
		cfg := acp.AgentConfig{
			ProviderMode:  acp.AgentProviderModeAgentDefaults,
			ModelProvider: stored.ModelProvider,
			Model:         stored.Model,
		}.NormalizeProviderModel(seed.ModelProvider)
		stored.ModelProvider = cfg.ModelProvider
		stored.Model = cfg.Model
		if strings.TrimSpace(stored.Model) == "" {
			stored.Model = seed.Model
		}
	}
	return stored
}

func canonicalizeACPDefaults(in map[string]ACPAgentDefaults) map[string]ACPAgentDefaults {
	out := map[string]ACPAgentDefaults{}
	for name, agent := range in {
		canonical := acp.CanonicalAgentName(name)
		if canonical == "" || canonical != strings.TrimSpace(name) {
			continue
		}
		out[canonical] = agent
	}
	for name, agent := range in {
		canonical := acp.CanonicalAgentName(name)
		if canonical == "" || canonical == strings.TrimSpace(name) {
			continue
		}
		if _, ok := out[canonical]; !ok {
			out[canonical] = agent
		}
	}
	return out
}

type ACPConfigSource struct {
	store     storage.SettingsStorage
	catalog   acp.AgentCatalog
	validator ReasoningEffortValidator
}

func NewACPConfigSource(store storage.SettingsStorage, catalog acp.AgentCatalog, validators ...ReasoningEffortValidator) *ACPConfigSource {
	var validator ReasoningEffortValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return &ACPConfigSource{store: store, catalog: catalog, validator: validator}
}

func (s *ACPConfigSource) effectiveDefaults() (AgentDefaults, error) {
	stored, err := LoadAgentDefaults(s.store)
	if err != nil {
		return AgentDefaults{}, err
	}
	seed := AgentDefaultsFromCatalog(s.catalog)
	return MergeAgentDefaults(stored, seed, s.catalog.Names()), nil
}

func (s *ACPConfigSource) AgentConfig(name string) (acp.AgentConfig, bool, error) {
	name = acp.CanonicalAgentName(name)
	cfg, ok := s.catalog.Agent(name)
	if !ok {
		return acp.AgentConfig{}, false, nil
	}
	defaults, err := s.effectiveDefaults()
	if err != nil {
		return acp.AgentConfig{}, false, err
	}
	agent, ok := defaults.ACP[name]
	if !ok || !agent.Enabled {
		return acp.AgentConfig{}, false, nil
	}
	// cfg already carries the catalog's launch command/args; the launch command is
	// catalog-owned, never user settings.
	defaultModelProvider := strings.TrimSpace(cfg.ModelProvider)
	cfg.ModelProvider = strings.TrimSpace(agent.ModelProvider)
	cfg.Model = strings.TrimSpace(agent.Model)
	cfg.ReasoningEffort = strings.TrimSpace(agent.ReasoningEffort)
	cfg.Auth = agent.Auth
	if cfg.UsesModelProvider() {
		cfg = cfg.NormalizeProviderModel(defaultModelProvider)
	}
	if s.validator != nil {
		if err := s.validator.ValidateReasoningEffort(name, cfg.ModelProvider, cfg.Model, cfg.ReasoningEffort); err != nil {
			return acp.AgentConfig{}, false, err
		}
	}
	return cfg, true, nil
}

func (s *ACPConfigSource) EnabledAgentNames() ([]string, error) {
	defaults, err := s.effectiveDefaults()
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, name := range s.catalog.Names() {
		agent, ok := defaults.ACP[name]
		if !ok || !agent.Enabled {
			continue
		}
		names = append(names, name)
	}
	return names, nil
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
