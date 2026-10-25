package acp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/musecli"
)

type AgentModelCatalog struct {
	Models       []AgentModel `json:"models"`
	DefaultModel string       `json:"default_model,omitempty"`
}

type agentMetadataSource interface {
	MetadataConfig(string) (AgentConfig, bool, error)
}

func (m *Manager) DiscoverModels(ctx context.Context, agent string) (AgentModelCatalog, error) {
	agent = CanonicalAgentName(agent)
	lookup := m.agents.AgentConfig
	if source, ok := m.agents.(agentMetadataSource); ok {
		lookup = source.MetadataConfig
	}
	cfg, ok, err := lookup(agent)
	if err != nil {
		return AgentModelCatalog{}, err
	}
	if !ok {
		return AgentModelCatalog{}, fmt.Errorf("unknown acp agent %q", agent)
	}
	if agent == AgentAntigravity {
		return antigravityModels(ctx, m.probeEnv(agent, cfg))
	}
	if agent == AgentKimi {
		return m.kimiModels(ctx, cfg)
	}
	if agent != AgentMuse {
		return AgentModelCatalog{Models: (ModelCapabilities{Catalog: m.cfg.ModelCatalog}).AgentModels(agent), DefaultModel: cfg.Model}, nil
	}
	env := m.probeEnv(agent, cfg)
	command, err := musecli.Resolve(env, "")
	if err != nil {
		return AgentModelCatalog{}, err
	}
	raw, err := museCall(ctx, command, env, "model/list")
	if err != nil {
		return AgentModelCatalog{}, err
	}
	var catalog struct {
		Models []struct {
			ModelID                string          `json:"modelId"`
			DisplayLabel           string          `json:"displayLabel"`
			Description            string          `json:"description"`
			ContextLimit           int             `json:"contextLimit"`
			IsDefault              bool            `json:"isDefault"`
			Variants               json.RawMessage `json:"variants"`
			DefaultReasoningEffort string          `json:"defaultReasoningEffort"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return AgentModelCatalog{}, fmt.Errorf("decode Muse models: %w", err)
	}
	result := AgentModelCatalog{Models: make([]AgentModel, 0, len(catalog.Models))}
	for _, native := range catalog.Models {
		model := AgentModel{
			Value: native.ModelID, Label: native.DisplayLabel, Description: native.Description, ContextLength: native.ContextLimit,
			Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
		}
		var efforts []string
		if json.Unmarshal(native.Variants, &efforts) == nil && efforts != nil {
			model.Reasoning.Status = modelcatalog.ReasoningReady
			model.Reasoning.Efforts = efforts
			model.Reasoning.DefaultEffort = native.DefaultReasoningEffort
		}
		result.Models = append(result.Models, model)
		if native.IsDefault {
			result.DefaultModel = native.ModelID
		}
	}
	return result, nil
}
