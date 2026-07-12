package acp

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

type ModelCapabilities struct {
	Catalog ModelCatalog
}

func (c ModelCapabilities) AgentModels(agent string) []modelcatalog.Model {
	return resolveModelCapabilities(agent, c.Catalog.AgentModels(agent), modelcatalog.ReasoningEffortScopeAgent)
}

func (c ModelCapabilities) AgentModelsForProvider(agent, providerID string) ([]modelcatalog.Model, error) {
	models, err := c.Catalog.CuratedAgentModelsForProvider(agent, providerID)
	if err != nil {
		return nil, err
	}
	fallbackScope := modelcatalog.ReasoningEffortScopeAgent
	if CanonicalAgentName(agent) == AgentOpenCode && !strings.EqualFold(providerID, provider.ProviderOpenRouter) {
		fallbackScope = ""
	}
	return resolveModelCapabilities(agent, models, fallbackScope), nil
}

func (c ModelCapabilities) ProviderModels(agent, providerID string) ([]modelcatalog.Model, error) {
	return ProviderModelCapabilities(c.Catalog, agent, providerID)
}

func ProviderModelCapabilities(catalog ProviderModelCatalog, agent, providerID string) ([]modelcatalog.Model, error) {
	models, err := catalog.ProviderModels(providerID)
	if err != nil {
		return nil, err
	}
	return resolveModelCapabilities(agent, models, ""), nil
}

func (c ModelCapabilities) ValidateReasoningEffort(agent, providerID, model, effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return nil
	}
	if found, ok := findCapabilityModel(c.AgentModels(agent), model); ok && found.ReasoningEffortsKnown {
		return validateModelReasoningEffort(agent, model, effort, found.ReasoningEfforts)
	}
	if strings.TrimSpace(providerID) == "" {
		return nil
	}
	models, err := c.ProviderModels(agent, providerID)
	if errors.Is(err, modelcatalog.ErrCatalogUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}
	if found, ok := findCapabilityModel(models, model); ok && found.ReasoningEffortsKnown {
		return validateModelReasoningEffort(agent, model, effort, found.ReasoningEfforts)
	}
	return nil
}

func resolveModelCapabilities(agent string, models []modelcatalog.Model, fallbackScope modelcatalog.ReasoningEffortScope) []modelcatalog.Model {
	agent = CanonicalAgentName(agent)
	policy := agentPolicyForAgent(agent)
	supported := reasoningEffortValues(policy.reasoningEffortOptions())
	for i := range models {
		model := &models[i]
		if !model.ReasoningEffortsKnown {
			if fallbackScope == "" || model.OpenRouterID != "" || strings.Contains(model.Value, "/") {
				model.ReasoningEfforts = nil
				continue
			}
			model.ReasoningEfforts = append([]string(nil), supported...)
			model.ReasoningEffortsKnown = true
			model.ReasoningEffortScope = fallbackScope
			continue
		}
		if model.ReasoningEffortScope == "" {
			model.ReasoningEffortScope = modelcatalog.ReasoningEffortScopeProvider
		}
		model.ReasoningEfforts = intersectReasoningEfforts(model.ReasoningEfforts, supported)
		if agent == AgentCodex && isCodexUltraModel(*model) {
			model.ReasoningEfforts = addReasoningEffort(model.ReasoningEfforts, "ultra")
		}
		if agent == AgentClaude && containsReasoningEffort(model.ReasoningEfforts, "xhigh") {
			model.ReasoningEfforts = addReasoningEffort(model.ReasoningEfforts, claudeReasoningEffortUltracode)
		}
		if model.ReasoningDefaultEffort != "" && !containsReasoningEffort(model.ReasoningEfforts, model.ReasoningDefaultEffort) {
			model.ReasoningDefaultEffort = ""
		}
	}
	return models
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
	values = append(values, value)
	sort.SliceStable(values, func(i, j int) bool { return reasoningEffortRank(values[i]) < reasoningEffortRank(values[j]) })
	return values
}

func containsReasoningEffort(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func reasoningEffortRank(value string) int {
	for i, current := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "ultracode"} {
		if current == value {
			return i
		}
	}
	return 100
}

func findCapabilityModel(models []modelcatalog.Model, value string) (modelcatalog.Model, bool) {
	value = strings.TrimSpace(value)
	if value == "" && len(models) > 0 {
		return models[0], true
	}
	for _, model := range models {
		if model.Value == value || model.OpenRouterID == value {
			return model, true
		}
	}
	return modelcatalog.Model{}, false
}

func validateModelReasoningEffort(agent, model, effort string, supported []string) error {
	if containsReasoningEffort(supported, effort) {
		return nil
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "default"
	}
	if len(supported) == 0 {
		return fmt.Errorf("reasoning effort %q is not supported for %s model %q", effort, strings.TrimSpace(agent), model)
	}
	return fmt.Errorf("reasoning effort %q is not supported for %s model %q; valid values are %s", effort, strings.TrimSpace(agent), model, strings.Join(supported, ", "))
}
