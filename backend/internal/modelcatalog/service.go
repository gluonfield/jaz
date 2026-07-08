package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/provider"
)

const codexOpenAIAPIKeyProvider = "openai-api-key"

var ErrCatalogUnavailable = errors.New("model provider catalog has not loaded")

type ProviderSource interface {
	Providers() map[string]provider.ModelProviderConfig
}

type Service struct {
	Providers ProviderSource

	warmMu         sync.Mutex
	mu             sync.RWMutex
	providerModels map[string][]Model
}

func NewService(providers ProviderSource) *Service {
	return &Service{Providers: providers}
}

func (s *Service) Warm(ctx context.Context) error {
	meta, ok := s.providerMeta(provider.ProviderOpenRouter)
	if !ok {
		return nil
	}
	key := modelCatalogKey(meta)
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	s.mu.RLock()
	_, loaded := s.providerModels[key]
	s.mu.RUnlock()
	if loaded {
		return nil
	}
	models, err := fetchOpenRouterModels(ctx, meta.BaseURL)
	if err != nil {
		return err
	}
	s.setProviderModels(meta, models)
	return nil
}

func (s *Service) ProviderModels(id string) ([]Model, error) {
	meta, ok := s.providerMeta(strings.ToLower(strings.TrimSpace(id)))
	if !ok {
		return nil, fmt.Errorf("unknown model provider %q", id)
	}
	switch strings.TrimSpace(meta.ID) {
	case provider.ProviderOpenAI, codexOpenAIAPIKeyProvider:
		return s.enrichReasoning(cloneModels(openAIModels)), nil
	case provider.ProviderOpenRouter:
		models, ok := s.providerModelSnapshot(meta)
		if !ok {
			return nil, fmt.Errorf("%w for %q", ErrCatalogUnavailable, provider.ProviderOpenRouter)
		}
		return models, nil
	default:
		if model := strings.TrimSpace(meta.DefaultModel); model != "" {
			return []Model{{Value: model, Label: model}}, nil
		}
		return []Model{}, nil
	}
}

func (s *Service) AgentModels(agent string) []Model {
	agent = strings.ToLower(strings.TrimSpace(agent))
	models := s.enrichReasoning(cloneModels(agentModels[agent]))
	if agent == "claude" {
		for i := range models {
			models[i].ReasoningEfforts = withUltracode(models[i].ReasoningEfforts)
		}
	}
	return models
}

func (s *Service) AgentModelsForProvider(agent, providerID string) ([]Model, error) {
	agent = strings.ToLower(strings.TrimSpace(agent))
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if agent == "opencode" && providerID != "" && providerID != provider.ProviderOpenRouter {
		return s.ProviderModels(providerID)
	}
	return s.AgentModels(agent), nil
}

func (s *Service) enrichReasoning(models []Model) []Model {
	for i := range models {
		source, ok := s.openRouterModel(models[i].OpenRouterID)
		if !ok {
			continue
		}
		models[i].ReasoningEfforts = source.ReasoningEfforts
		models[i].ReasoningDefaultEffort = source.ReasoningDefaultEffort
		models[i].ReasoningMandatory = source.ReasoningMandatory
	}
	return models
}

func (s *Service) openRouterModel(id string) (Model, bool) {
	if id == "" {
		return Model{}, false
	}
	meta, ok := s.providerMeta(provider.ProviderOpenRouter)
	if !ok {
		return Model{}, false
	}
	key := modelCatalogKey(meta)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, model := range s.providerModels[key] {
		if model.Value == id {
			return cloneModel(model), true
		}
	}
	return Model{}, false
}

func (s *Service) ValidateReasoningEffort(agent, providerID, model, effort string) error {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return nil
	}
	efforts, ok, err := s.reasoningEfforts(agent, providerID, model)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	for _, allowed := range efforts {
		if effort == allowed {
			return nil
		}
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "default"
	}
	if len(efforts) == 0 {
		return fmt.Errorf("reasoning effort %q is not supported for %s model %q", effort, strings.TrimSpace(agent), model)
	}
	return fmt.Errorf("reasoning effort %q is not supported for %s model %q; valid values are %s", effort, strings.TrimSpace(agent), model, strings.Join(efforts, ", "))
}

func (s *Service) reasoningEfforts(agent, providerID, model string) ([]string, bool, error) {
	agent = strings.ToLower(strings.TrimSpace(agent))
	if model, ok := findModel(s.AgentModels(agent), model); ok {
		if model.ReasoningEfforts == nil {
			if efforts := harnessReasoningEfforts(agent); efforts != nil {
				return efforts, true, nil
			}
		} else {
			return cloneStrings(model.ReasoningEfforts), true, nil
		}
	} else if efforts := harnessReasoningEfforts(agent); efforts != nil {
		return efforts, true, nil
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == codexOpenAIAPIKeyProvider {
		providerID = provider.ProviderOpenAI
	}
	if providerID == "" {
		return nil, false, nil
	}
	models, err := s.ProviderModels(providerID)
	if err != nil {
		return nil, false, err
	}
	found, ok := findModel(models, model)
	if !ok || found.ReasoningEfforts == nil {
		return nil, false, nil
	}
	return cloneStrings(found.ReasoningEfforts), true, nil
}

func harnessReasoningEfforts(agent string) []string {
	switch agent {
	case "codex":
		return cloneStrings(codexHarnessEfforts)
	case "claude":
		return cloneStrings(claudeHarnessEfforts)
	default:
		return nil
	}
}

func findModel(models []Model, value string) (Model, bool) {
	value = strings.TrimSpace(value)
	if value == "" && len(models) > 0 {
		return models[0], true
	}
	for _, model := range models {
		if model.Value == value || model.OpenRouterID == value {
			return model, true
		}
	}
	return Model{}, false
}

func (s *Service) providerMeta(id string) (provider.ModelProvider, bool) {
	if id == "" {
		return provider.ModelProvider{}, false
	}
	key := id
	if id == codexOpenAIAPIKeyProvider {
		key = provider.ProviderOpenAI
	}
	cfg := map[string]provider.ModelProviderConfig{}
	if s != nil && s.Providers != nil {
		cfg = s.Providers.Providers()
	}
	meta, ok := provider.ModelProviderByID(key)
	if !ok {
		if !provider.ModelProviderConfigPresent(cfg[key]) {
			return provider.ModelProvider{}, false
		}
		meta = provider.ModelProvider{ID: key}
	}
	meta = provider.ApplyModelProviderConfig(meta, cfg[key])
	if id == codexOpenAIAPIKeyProvider {
		meta.ID = codexOpenAIAPIKeyProvider
	}
	return meta, true
}

func (s *Service) providerModelSnapshot(meta provider.ModelProvider) ([]Model, bool) {
	key := modelCatalogKey(meta)
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.providerModels[key]
	if !ok {
		return nil, false
	}
	return cloneModels(entry), true
}

func (s *Service) setProviderModels(meta provider.ModelProvider, models []Model) {
	key := modelCatalogKey(meta)
	s.mu.Lock()
	if s.providerModels == nil {
		s.providerModels = map[string][]Model{}
	}
	s.providerModels[key] = cloneModels(models)
	s.mu.Unlock()
}

func modelCatalogKey(meta provider.ModelProvider) string {
	return strings.TrimSpace(meta.ID) + " " + strings.TrimRight(strings.TrimSpace(meta.BaseURL), "/")
}
