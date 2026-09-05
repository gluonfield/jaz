package acp

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

var ErrReasoningCapabilitiesUnavailable = errors.New("reasoning capabilities are unavailable")

const defaultGrokReasoningEffort = "high"

type ReasoningScope string

const (
	ReasoningScopeProvider ReasoningScope = "provider"
	ReasoningScopeAgent    ReasoningScope = "agent"
)

type ReasoningCapabilities struct {
	Status        modelcatalog.ReasoningStatus `json:"status"`
	Scope         ReasoningScope               `json:"scope,omitempty"`
	Efforts       []string                     `json:"efforts,omitempty"`
	DefaultEffort string                       `json:"default_effort,omitempty"`
	Mandatory     bool                         `json:"mandatory,omitempty"`
	Automatic     bool                         `json:"automatic,omitempty"`
}

type AgentModel struct {
	Value           string                `json:"value"`
	Label           string                `json:"label"`
	Aliases         []string              `json:"aliases,omitempty"`
	Description     string                `json:"description,omitempty"`
	ContextLength   int                   `json:"context_length,omitempty"`
	InputModalities []string              `json:"-"`
	Pricing         *modelcatalog.Pricing `json:"pricing,omitempty"`
	OpenRouterID    string                `json:"openrouter_id,omitempty"`
	Reasoning       ReasoningCapabilities `json:"reasoning"`
}

type ModelCapabilities struct {
	Catalog ModelCatalog
}

func (c ModelCapabilities) AgentModels(agent string) []AgentModel {
	return c.curatedModels(agent, "")
}

func (c ModelCapabilities) AgentModelsForProvider(agent, providerID string) ([]AgentModel, error) {
	if !usesCuratedModels(agent, providerID) {
		return c.ProviderModels(agent, providerID)
	}
	return c.curatedModels(agent, providerID), nil
}

func (c ModelCapabilities) curatedModels(agent, providerID string) []AgentModel {
	return resolveModelCapabilities(agent, c.Catalog.AgentModels(agent), agentOwnsModelMetadata(agent, providerID))
}

// usesCuratedModels reports whether Jaz's own model list describes the pair,
// either because the agent owns the metadata or because Jaz curates that
// provider's models for it.
func usesCuratedModels(agent, providerID string) bool {
	if agentOwnsModelMetadata(agent, providerID) {
		return true
	}
	switch CanonicalAgentName(agent) {
	case AgentCodex, AgentOpenCode:
		return strings.ToLower(strings.TrimSpace(providerID)) == provider.ProviderOpenRouter
	}
	return false
}

func (c ModelCapabilities) ProviderModels(agent, providerID string) ([]AgentModel, error) {
	models, err := c.Catalog.ProviderModels(providerID)
	if err != nil {
		return nil, err
	}
	resolved := resolveModelCapabilities(agent, models, agentOwnsModelMetadata(agent, providerID))
	addModelAliases(resolved, c.Catalog.AgentModels(agent))
	return resolved, nil
}

func addModelAliases(models []AgentModel, curated []modelcatalog.Model) {
	for i := range models {
		id := modelIdentity(models[i].Value, models[i].OpenRouterID)
		for _, candidate := range curated {
			if id != modelIdentity(candidate.Value, candidate.OpenRouterID) {
				continue
			}
			models[i].Aliases = addModelAlias(models[i].Aliases, models[i].Value, candidate.Value)
			models[i].Aliases = addModelAlias(models[i].Aliases, models[i].Value, candidate.Label)
			models[i].Aliases = addModelAlias(models[i].Aliases, models[i].Value, candidate.OpenRouterID)
		}
	}
}

func modelIdentity(value, openRouterID string) string {
	if openRouterID != "" {
		return strings.ToLower(strings.TrimSpace(openRouterID))
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func addModelAlias(aliases []string, value, alias string) []string {
	alias = strings.TrimSpace(alias)
	if alias == "" || alias == value || containsString(aliases, alias) {
		return aliases
	}
	return append(aliases, alias)
}

// agentOwnsModelMetadata reports whether the agent supplies the model catalog
// and reasoning capabilities itself instead of Jaz's provider catalog: with no
// provider selected every agent runs on its own defaults, and Codex keeps its
// built-in catalog on its own backend and with an OpenAI API key.
func agentOwnsModelMetadata(agent, providerID string) bool {
	if strings.TrimSpace(providerID) == "" {
		return true
	}
	if CanonicalAgentName(agent) != AgentCodex {
		return false
	}
	return codexNativeOpenAIProvider(providerID) ||
		strings.EqualFold(strings.TrimSpace(providerID), CodexProviderOpenAIAPIKey)
}

// reasoningCapabilityModels returns the models a configured effort is judged
// against. A selected provider's capabilities win even where the picker offers
// Jaz's curated list, so that an effort is never accepted against a catalog the
// turn will not run on.
func (c ModelCapabilities) reasoningCapabilityModels(agent, providerID string) ([]AgentModel, error) {
	if strings.TrimSpace(providerID) == "" {
		return c.AgentModels(agent), nil
	}
	return c.ProviderModels(agent, providerID)
}

func (c ModelCapabilities) ValidateReasoningEffort(agent, providerID, model, effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return nil
	}
	models, err := c.reasoningCapabilityModels(agent, providerID)
	if err != nil {
		return err
	}
	found, ok := findCapabilityModel(models, model)
	if !ok && strings.TrimSpace(providerID) == "" && agentPolicyForAgent(agent).supportsReasoningEffort(effort) {
		return nil
	}
	if !ok || found.Reasoning.Status == modelcatalog.ReasoningUnavailable {
		return fmt.Errorf("%w for %s model %q", ErrReasoningCapabilitiesUnavailable, strings.TrimSpace(agent), displayModel(model))
	}
	if found.Reasoning.Status == modelcatalog.ReasoningPending {
		return modelcatalog.ErrCatalogUnavailable
	}
	return validateModelReasoningEffort(agent, model, effort, found.Reasoning.Efforts)
}

func resolveModelCapabilities(agent string, models []modelcatalog.Model, allowAgentCapabilities bool) []AgentModel {
	agent = CanonicalAgentName(agent)
	supported := reasoningEffortValues(agentPolicyForAgent(agent).reasoningEffortOptions())
	out := make([]AgentModel, 0, len(models))
	for _, model := range models {
		resolved := AgentModel{
			Value:           model.Value,
			Label:           model.Label,
			Description:     model.Description,
			ContextLength:   model.ContextLength,
			InputModalities: append([]string(nil), model.InputModalities...),
			Pricing:         model.Pricing,
			OpenRouterID:    model.OpenRouterID,
			Reasoning: ReasoningCapabilities{
				Status: model.Reasoning.Status,
			},
		}
		switch model.Reasoning.Status {
		case modelcatalog.ReasoningReady:
			resolved.Reasoning.Scope = ReasoningScopeProvider
			resolved.Reasoning.Efforts = intersectReasoningEfforts(model.Reasoning.Efforts, supported)
			resolved.Reasoning.DefaultEffort = model.Reasoning.DefaultEffort
			resolved.Reasoning.Mandatory = model.Reasoning.Mandatory
			resolved.Reasoning.Automatic = model.Reasoning.Automatic
			if agent == AgentClaude && containsString(resolved.Reasoning.Efforts, "xhigh") {
				resolved.Reasoning.Efforts = addReasoningEffort(resolved.Reasoning.Efforts, claudeReasoningEffortUltracode)
			}
		case modelcatalog.ReasoningUnavailable, modelcatalog.ReasoningPending:
			if allowAgentCapabilities {
				resolved.Reasoning = agentReasoningCapabilities(agent, model, supported)
			}
		}
		if resolved.Reasoning.Status == modelcatalog.ReasoningReady {
			if agent == AgentCodex {
				if isCodexUltraModel(model) {
					resolved.Reasoning.Efforts = addReasoningEffort(resolved.Reasoning.Efforts, "ultra")
				} else {
					resolved.Reasoning.Efforts = slices.DeleteFunc(resolved.Reasoning.Efforts, func(value string) bool { return value == "ultra" })
				}
			}
			if !containsString(resolved.Reasoning.Efforts, resolved.Reasoning.DefaultEffort) {
				resolved.Reasoning.DefaultEffort = ""
			}
		}
		out = append(out, resolved)
	}
	return out
}

func agentReasoningCapabilities(agent string, model modelcatalog.Model, supported []string) ReasoningCapabilities {
	capabilities := ReasoningCapabilities{
		Status:  modelcatalog.ReasoningReady,
		Scope:   ReasoningScopeAgent,
		Efforts: append([]string(nil), supported...),
	}
	if agent != AgentGrok {
		return capabilities
	}
	switch model.Value {
	case modelcatalog.DefaultGrokModel, modelcatalog.GrokLegacyModel:
		capabilities.Efforts = []string{"low", "medium", defaultGrokReasoningEffort}
		capabilities.DefaultEffort = defaultGrokReasoningEffort
	case modelcatalog.GrokComposerModel:
		capabilities.Efforts = []string{}
	}
	return capabilities
}

func isCodexUltraModel(model modelcatalog.Model) bool {
	id := model.OpenRouterID
	if id == "" {
		id = model.Value
	}
	switch id {
	case provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT6Astra,
		provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT56Sol,
		provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT56Terra,
		provider.OpenAIModelGPT6Astra,
		provider.OpenAIModelGPT56Sol,
		provider.OpenAIModelGPT56Terra:
		return true
	}
	return false
}

func intersectReasoningEfforts(values, supported []string) []string {
	allowed := make(map[string]struct{}, len(supported))
	for _, value := range supported {
		allowed[value] = struct{}{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func addReasoningEffort(values []string, value string) []string {
	if containsString(values, value) {
		return values
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func findCapabilityModel(models []AgentModel, value string) (AgentModel, bool) {
	value = strings.TrimSpace(value)
	if value == "" && len(models) > 0 {
		return models[0], true
	}
	for _, model := range models {
		if model.Value == value || model.OpenRouterID == value || containsString(model.Aliases, value) {
			return model, true
		}
	}
	return AgentModel{}, false
}

func validateModelReasoningEffort(agent, model, effort string, supported []string) error {
	if containsString(supported, effort) {
		return nil
	}
	if len(supported) == 0 {
		return fmt.Errorf("reasoning effort %q is not supported for %s model %q", effort, strings.TrimSpace(agent), displayModel(model))
	}
	return fmt.Errorf("reasoning effort %q is not supported for %s model %q; valid values are %s", effort, strings.TrimSpace(agent), displayModel(model), strings.Join(supported, ", "))
}

func displayModel(model string) string {
	if model = strings.TrimSpace(model); model != "" {
		return model
	}
	return "default"
}
