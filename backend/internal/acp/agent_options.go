package acp

import (
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

const agentOptionsProviderModelLimit = 10

type AgentOptionsRequest struct {
	Agent string `json:"agent,omitempty"`
	Name  string `json:"name,omitempty"`
}

type AgentOptionsOutput struct {
	Agents       []string                `json:"agents"`
	AgentOptions map[string]AgentOptions `json:"agent_options,omitempty"`
}

func (m *Manager) AgentOptions(req AgentOptionsRequest) AgentOptionsOutput {
	agent := CanonicalAgentName(req.Agent)
	agents := SelectableAgentNames(m.Agents())
	if agent != "" {
		agents = filterAgentNames(agents, agent)
	}
	return AgentOptionsOutput{
		Agents:       agents,
		AgentOptions: m.agentOptions(agents, req.Name),
	}
}

func (m *Manager) agentOptions(agents []string, query string) map[string]AgentOptions {
	out := make(map[string]AgentOptions, len(agents))
	for _, agent := range agents {
		cfg, ok, err := m.configuredAgent(agent)
		if err != nil || !ok {
			continue
		}
		option := AgentOptionsForConfig(agent, cfg)
		option.ModelProviderIDs = m.agentModelProviderIDs(agent, cfg)
		if m.cfg.ModelCatalog != nil {
			option.Models = m.agentModelOptions(agent, cfg, query)
		}
		out[agent] = option
	}
	return out
}

func (m *Manager) agentModelOptions(agent string, cfg AgentConfig, query string) []modelcatalog.Model {
	models := filterModels(m.cfg.ModelCatalog.AgentModels(agent), query)
	if !m.agentSupportsModelProvider(agent, cfg, provider.ProviderOpenRouter) || strings.TrimSpace(query) == "" {
		return models
	}
	providerModels, err := m.cfg.ModelCatalog.ProviderModels(provider.ProviderOpenRouter)
	if err != nil {
		return models
	}
	seen := map[string]struct{}{}
	for _, model := range models {
		seen[model.Value] = struct{}{}
	}
	for _, model := range filterModels(providerModels, query) {
		if len(models) >= agentOptionsProviderModelLimit {
			break
		}
		if _, ok := seen[model.Value]; ok {
			continue
		}
		models = append(models, model)
		seen[model.Value] = struct{}{}
	}
	return models
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

func filterModels(models []modelcatalog.Model, query string) []modelcatalog.Model {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return models
	}
	out := []modelcatalog.Model{}
	for _, model := range models {
		if modelMatches(model, query) {
			out = append(out, model)
		}
	}
	return out
}

func modelMatches(model modelcatalog.Model, query string) bool {
	return strings.Contains(strings.ToLower(model.Value), query) ||
		strings.Contains(strings.ToLower(model.Label), query) ||
		strings.Contains(strings.ToLower(model.Description), query) ||
		strings.Contains(strings.ToLower(model.OpenRouterID), query)
}
