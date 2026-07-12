package acp

import (
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

const agentOptionsProviderModelLimit = 10

type AgentOptionsRequest struct {
	Agent string `json:"agent,omitempty"`
	Name  string `json:"name,omitempty"`
}

type AgentOptionsOutput struct {
	Agents []AgentSpawnOptions `json:"agents"`
}

type AgentSpawnOptions struct {
	Name                   string             `json:"name"`
	DefaultModel           string             `json:"default_model,omitempty"`
	DefaultModelProvider   string             `json:"default_model_provider,omitempty"`
	DefaultReasoningEffort string             `json:"default_reasoning_effort,omitempty"`
	Models                 []AgentModelOption `json:"models,omitempty"`
	ModelSearch            *AgentModelSearch  `json:"model_search,omitempty"`
}

type AgentModelOption struct {
	Model         string                `json:"model"`
	Label         string                `json:"label,omitempty"`
	ModelProvider string                `json:"model_provider,omitempty"`
	ContextLength int                   `json:"context_length,omitempty"`
	Reasoning     ReasoningCapabilities `json:"reasoning"`
}

type AgentModelSearch struct {
	Provider string `json:"provider"`
	Limit    int    `json:"limit"`
	Use      string `json:"use"`
}

func (m *Manager) AgentOptions(req AgentOptionsRequest) (AgentOptionsOutput, error) {
	agent := CanonicalAgentName(req.Agent)
	names, err := m.agents.EnabledAgentNames()
	if err != nil {
		return AgentOptionsOutput{}, err
	}
	agents := SelectableAgentNames(names)
	if agent != "" {
		agents = filterAgentNames(agents, agent)
	}
	options, err := m.agentOptions(agents, req.Name)
	if err != nil {
		return AgentOptionsOutput{}, err
	}
	return AgentOptionsOutput{Agents: options}, nil
}

func (m *Manager) agentOptions(agents []string, query string) ([]AgentSpawnOptions, error) {
	out := make([]AgentSpawnOptions, 0, len(agents))
	for _, agent := range agents {
		cfg, ok, err := m.configuredAgent(agent)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		option := AgentSpawnOptions{
			Name:                   agent,
			DefaultModel:           strings.TrimSpace(cfg.Model),
			DefaultModelProvider:   strings.TrimSpace(cfg.ModelProvider),
			DefaultReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
		}
		if m.cfg.ModelCatalog != nil {
			models, err := m.agentModelOptions(agent, cfg, query)
			if err != nil {
				return nil, err
			}
			option.Models = models
			if m.agentSupportsModelProvider(agent, cfg, provider.ProviderOpenRouter) {
				option.ModelSearch = &AgentModelSearch{
					Provider: provider.ProviderOpenRouter,
					Limit:    agentOptionsProviderModelLimit,
					Use:      `agent_options({"agent":"` + agent + `","name":"<model name or provider>"})`,
				}
			}
		}
		out = append(out, option)
	}
	return out, nil
}

func (m *Manager) agentModelOptions(agent string, cfg AgentConfig, query string) ([]AgentModelOption, error) {
	catalogModels, err := (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).AgentModelsForProvider(agent, cfg.ModelProvider)
	if err != nil {
		return nil, err
	}
	models := modelOptionsForCatalogModels(cfg, filterModels(catalogModels, query), "")
	if !m.agentSupportsModelProvider(agent, cfg, provider.ProviderOpenRouter) || strings.TrimSpace(query) == "" {
		return models, nil
	}
	providerModels, err := (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).ProviderModels(agent, provider.ProviderOpenRouter)
	if err != nil {
		if len(models) == 0 {
			return nil, err
		}
		return models, nil
	}
	seen := map[string]struct{}{}
	for _, model := range models {
		seen[model.Model] = struct{}{}
	}
	for _, model := range filterModels(providerModels, query) {
		if len(models) >= agentOptionsProviderModelLimit {
			break
		}
		if _, ok := seen[model.Value]; ok {
			continue
		}
		models = append(models, modelOptionForCatalogModel(cfg, model, provider.ProviderOpenRouter))
		seen[model.Value] = struct{}{}
	}
	return models, nil
}

func modelOptionsForCatalogModels(cfg AgentConfig, models []AgentModel, sourceProvider string) []AgentModelOption {
	out := make([]AgentModelOption, 0, len(models))
	for _, model := range models {
		out = append(out, modelOptionForCatalogModel(cfg, model, sourceProvider))
	}
	return out
}

func modelOptionForCatalogModel(cfg AgentConfig, model AgentModel, sourceProvider string) AgentModelOption {
	option := AgentModelOption{
		Model:         strings.TrimSpace(model.Value),
		Label:         strings.TrimSpace(model.Label),
		ContextLength: model.ContextLength,
		Reasoning:     model.Reasoning,
	}
	if providerID := strings.TrimSpace(sourceProvider); providerID != "" && !strings.EqualFold(providerID, strings.TrimSpace(cfg.ModelProvider)) {
		option.ModelProvider = providerID
	}
	return option
}

func (m *Manager) agentSupportsModelProvider(agent string, cfg AgentConfig, providerID string) bool {
	for _, id := range m.agentModelProviderIDs(agent, cfg) {
		if strings.EqualFold(id, providerID) {
			return true
		}
	}
	return false
}

func (m *Manager) agentModelProviderIDs(agent string, cfg AgentConfig) []string {
	if !cfg.UsesProvider() {
		return nil
	}
	providers := m.effectiveModelProviders()
	if CanonicalAgentName(agent) == AgentCodex && cfg.ModelProviderCapability == provider.CapabilityCodex {
		return codexModelProviderIDs(providers)
	}
	ids := []string{}
	for _, modelProvider := range providers {
		if modelProvider.SupportsCapability(cfg.ModelProviderCapability) {
			ids = append(ids, modelProvider.ID)
		}
	}
	return ids
}

func (m *Manager) effectiveModelProviders() []provider.ModelProvider {
	providerConfig := m.providers()
	out := provider.ModelProviders()
	seen := map[string]int{}
	for i := range out {
		seen[out[i].ID] = i
		if cfg, ok := providerConfig[out[i].ID]; ok {
			out[i] = provider.ApplyModelProviderConfig(out[i], cfg)
		}
	}
	extra := []provider.ModelProvider{}
	for id, cfg := range providerConfig {
		if _, ok := seen[id]; ok || !provider.ModelProviderConfigPresent(cfg) {
			continue
		}
		extra = append(extra, provider.ApplyModelProviderConfig(provider.ModelProvider{ID: id}, cfg))
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].ID < extra[j].ID })
	return append(out, extra...)
}

func codexModelProviderIDs(providers []provider.ModelProvider) []string {
	ids := []string{}
	for _, modelProvider := range providers {
		if modelProvider.ID == provider.ProviderOpenAI {
			ids = append(ids, provider.ProviderOpenAI, CodexProviderOpenAIAPIKey)
			break
		}
	}
	for _, modelProvider := range providers {
		if modelProvider.ID != provider.ProviderOpenAI && modelProvider.SupportsCapability(provider.CapabilityCodex) {
			ids = append(ids, modelProvider.ID)
		}
	}
	return ids
}

func filterAgentNames(agents []string, agent string) []string {
	for _, name := range agents {
		if name == agent {
			return []string{name}
		}
	}
	return nil
}

func filterModels(models []AgentModel, query string) []AgentModel {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return models
	}
	out := []AgentModel{}
	for _, model := range models {
		if modelMatches(model, query) {
			out = append(out, model)
		}
	}
	return out
}

func modelMatches(model AgentModel, query string) bool {
	return strings.Contains(strings.ToLower(model.Value), query) ||
		strings.Contains(strings.ToLower(model.Label), query) ||
		strings.Contains(strings.ToLower(model.Description), query) ||
		strings.Contains(strings.ToLower(model.OpenRouterID), query)
}
