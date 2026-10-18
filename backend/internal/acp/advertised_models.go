package acp

import (
	"slices"
	"strings"
)

type advertisedModelsKey struct {
	agent    string
	provider string
}

// rememberAdvertisedModels keeps the model list from an agent's latest session
// where that list decides which model a session runs: the agent owns its
// models and Jaz refuses or skips the ones it does not offer.
func (m *Manager) rememberAdvertisedModels(agent string, cfg AgentConfig, state sessionModelState) {
	if key, ok := advertisedModelsKeyFor(agent, cfg); ok && !state.empty() {
		m.advertisedModels.Store(key, state)
	}
}

// startingModel is the model a new session for cfg would run, judged by the
// session-start check against the agent's latest advertisement. Until the
// agent has advertised, the configured model is taken as is.
func (m *Manager) startingModel(agent string, cfg AgentConfig) (string, error) {
	policy := agentPolicyForAgent(agent)
	return policy.sessionModelToSend(agent, policy.sessionConfigModel(cfg), m.latestAdvertisedModels(agent, cfg), cfg.ModelRequested)
}

// startableModels drops the models a request naming them would be refused.
func (m *Manager) startableModels(agent string, cfg AgentConfig, models []AgentModel) []AgentModel {
	return slices.DeleteFunc(models, func(model AgentModel) bool {
		candidate := cfg
		candidate.Model = model.Value
		candidate.ModelRequested = true
		_, err := m.startingModel(agent, candidate)
		return err != nil
	})
}

func (m *Manager) latestAdvertisedModels(agent string, cfg AgentConfig) sessionModelState {
	key, ok := advertisedModelsKeyFor(agent, cfg)
	if !ok {
		return sessionModelState{}
	}
	value, _ := m.advertisedModels.Load(key)
	state, _ := value.(sessionModelState)
	return state
}

func advertisedModelsKeyFor(agent string, cfg AgentConfig) (advertisedModelsKey, bool) {
	agent = CanonicalAgentName(agent)
	if !agentOwnsModelMetadata(agent, cfg.ModelProvider) || agentPolicyForAgent(agent).unadvertisedModel == unadvertisedModelSent {
		return advertisedModelsKey{}, false
	}
	return advertisedModelsKey{agent: agent, provider: strings.ToLower(strings.TrimSpace(cfg.ModelProvider))}, true
}
