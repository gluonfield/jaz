package acp

import (
	"cmp"
	"slices"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func kimiModels(home string) (AgentModelCatalog, error) {
	cfg, ok := readKimiConfig(home)
	if !ok {
		return AgentModelCatalog{}, errKimiModelNotConfigured
	}
	catalog := AgentModelCatalog{Models: make([]AgentModel, 0, len(cfg.Models)), DefaultModel: cfg.DefaultModel}
	for id, native := range cfg.Models {
		catalog.Models = append(catalog.Models, AgentModel{
			Value: id, Label: firstNonEmpty(native.DisplayName, native.Model, id), ContextLength: native.MaxContextSize,
			Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
		})
	}
	slices.SortFunc(catalog.Models, func(a, b AgentModel) int {
		return cmp.Compare(a.Value, b.Value)
	})
	return catalog, nil
}
