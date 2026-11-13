package acp

import (
	"context"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func copilotModels(ctx context.Context, env map[string]string) (AgentModelCatalog, error) {
	var native struct {
		Models []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Capabilities struct {
				Supports struct {
					ReasoningEffort *bool `json:"reasoningEffort"`
				} `json:"supports"`
				Limits struct {
					ContextWindow int `json:"max_context_window_tokens"`
				} `json:"limits"`
			} `json:"capabilities"`
			Efforts       []string `json:"supportedReasoningEfforts"`
			DefaultEffort string   `json:"defaultReasoningEffort"`
		} `json:"models"`
	}
	if err := copilotCall(ctx, env, "models.list", &native); err != nil {
		return AgentModelCatalog{}, err
	}
	catalog := AgentModelCatalog{Models: []AgentModel{}}
	for _, model := range native.Models {
		reasoning := ReasoningCapabilities{Status: modelcatalog.ReasoningPending, Scope: ReasoningScopeAgent}
		if supported := model.Capabilities.Supports.ReasoningEffort; supported != nil && !*supported {
			reasoning.Status = modelcatalog.ReasoningUnavailable
		}
		if len(model.Efforts) > 0 {
			reasoning.Status = modelcatalog.ReasoningReady
			reasoning.Efforts = model.Efforts
			reasoning.DefaultEffort = model.DefaultEffort
		}
		catalog.Models = append(catalog.Models, AgentModel{
			Value: model.ID, Label: model.Name, ContextLength: model.Capabilities.Limits.ContextWindow, Reasoning: reasoning,
		})
	}
	return catalog, nil
}
