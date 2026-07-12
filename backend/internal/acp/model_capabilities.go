package acp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

var ErrReasoningCapabilitiesUnavailable = errors.New("reasoning capabilities are unavailable")

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
}

type AgentModel struct {
	Value         string                `json:"value"`
	Label         string                `json:"label"`
	Description   string                `json:"description,omitempty"`
	ContextLength int                   `json:"context_length,omitempty"`
	Pricing       *modelcatalog.Pricing `json:"pricing,omitempty"`
	OpenRouterID  string                `json:"openrouter_id,omitempty"`
	Reasoning     ReasoningCapabilities `json:"reasoning"`
}

type ModelCapabilities struct {
	Catalog ModelCatalog
}

func (c ModelCapabilities) AgentModels(agent string) []AgentModel {
	return resolveModelCapabilities(agent, c.Catalog.AgentModels(agent), true)
}

func (c ModelCapabilities) AgentModelsForProvider(agent, providerID string) ([]AgentModel, error) {
	if !usesCuratedModels(agent, providerID) {
		return c.ProviderModels(agent, providerID)
	}
	return resolveModelCapabilities(agent, c.Catalog.AgentModels(agent), agentReasoningFallback(agent, providerID)), nil
}

func usesCuratedModels(agent, providerID string) bool {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return true
	}
	switch CanonicalAgentName(agent) {
	case AgentCodex:
		return providerID == provider.ProviderOpenAI || providerID == CodexProviderOpenAIAPIKey || providerID == provider.ProviderOpenRouter
	case AgentOpenCode:
		return providerID == provider.ProviderOpenRouter
	}
	return false
}

func (c ModelCapabilities) ProviderModels(agent, providerID string) ([]AgentModel, error) {
	return ProviderModelCapabilities(c.Catalog, agent, providerID)
}

func ProviderModelCapabilities(catalog ProviderModelCatalog, agent, providerID string) ([]AgentModel, error) {
	models, err := catalog.ProviderModels(providerID)
	if err != nil {
		return nil, err
	}
	return resolveModelCapabilities(agent, models, agentReasoningFallback(agent, providerID)), nil
}

func agentReasoningFallback(agent, providerID string) bool {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	return providerID == "" || CanonicalAgentName(agent) == AgentCodex &&
		(providerID == provider.ProviderOpenAI || providerID == CodexProviderOpenAIAPIKey)
}

func (c ModelCapabilities) ValidateReasoningEffort(agent, providerID, model, effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return nil
	}
	var (
		models []AgentModel
		err    error
	)
	if strings.TrimSpace(providerID) == "" {
		models = c.AgentModels(agent)
	} else {
		models, err = c.ProviderModels(agent, providerID)
		if err != nil {
			return err
		}
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

func resolveModelCapabilities(agent string, models []modelcatalog.Model, agentFallback bool) []AgentModel {
	agent = CanonicalAgentName(agent)
	supported := reasoningEffortValues(agentPolicyForAgent(agent).reasoningEffortOptions())
	out := make([]AgentModel, 0, len(models))
	for _, model := range models {
		resolved := AgentModel{
			Value:         model.Value,
			Label:         model.Label,
			Description:   model.Description,
			ContextLength: model.ContextLength,
			Pricing:       model.Pricing,
			OpenRouterID:  model.OpenRouterID,
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
			if agent == AgentCodex && isCodexUltraModel(model) {
				resolved.Reasoning.Efforts = addReasoningEffort(resolved.Reasoning.Efforts, "ultra")
			}
			if agent == AgentClaude && containsReasoningEffort(resolved.Reasoning.Efforts, "xhigh") {
				resolved.Reasoning.Efforts = addReasoningEffort(resolved.Reasoning.Efforts, claudeReasoningEffortUltracode)
			}
			if !containsReasoningEffort(resolved.Reasoning.Efforts, resolved.Reasoning.DefaultEffort) {
				resolved.Reasoning.DefaultEffort = ""
			}
		case modelcatalog.ReasoningUnavailable:
			if agentFallback && model.OpenRouterID == "" {
				resolved.Reasoning.Status = modelcatalog.ReasoningReady
				resolved.Reasoning.Scope = ReasoningScopeAgent
				resolved.Reasoning.Efforts = append([]string(nil), supported...)
			}
		}
		out = append(out, resolved)
	}
	return out
}

func isCodexUltraModel(model modelcatalog.Model) bool {
	id := model.OpenRouterID
	if id == "" {
		id = model.Value
	}
	switch id {
	case provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT56Sol,
		provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT56Terra,
		provider.ProviderOpenAI + "/" + provider.OpenAIModelGPT56Luna,
		provider.OpenAIModelGPT56Sol,
		provider.OpenAIModelGPT56Terra,
		provider.OpenAIModelGPT56Luna:
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
	if containsReasoningEffort(values, value) {
		return values
	}
	return append(values, value)
}

func containsReasoningEffort(values []string, value string) bool {
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
		if model.Value == value || model.OpenRouterID == value {
			return model, true
		}
	}
	return AgentModel{}, false
}

func validateModelReasoningEffort(agent, model, effort string, supported []string) error {
	if containsReasoningEffort(supported, effort) {
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
