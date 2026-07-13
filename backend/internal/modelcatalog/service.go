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
		if meta.OpenAICompatible && !meta.RequiresAPIKey {
			models, err := fetchOpenAICompatibleModels(context.Background(), meta.BaseURL)
			if err != nil {
				return nil, fmt.Errorf("%w for %q: %w", ErrCatalogUnavailable, meta.ID, err)
			}
			return models, nil
		}
		if model := strings.TrimSpace(meta.DefaultModel); model != "" {
			return []Model{{Value: model, Label: model, Reasoning: Reasoning{Status: ReasoningUnavailable}}}, nil
		}
		return []Model{}, nil
	}
}

func (s *Service) AgentModels(agent string) []Model {
	agent = strings.ToLower(strings.TrimSpace(agent))
	return s.enrichReasoning(cloneModels(agentModels[agent]))
}

func (s *Service) enrichReasoning(models []Model) []Model {
	sources, loaded := s.openRouterModels()
	if !loaded {
		return models
	}
	byID := make(map[string]Model, len(sources))
	for _, source := range sources {
		byID[source.Value] = source
	}
	for i := range models {
		if models[i].Reasoning.Status != ReasoningPending {
			continue
		}
		source, ok := byID[models[i].OpenRouterID]
		if !ok {
			models[i].Reasoning.Status = ReasoningUnavailable
			continue
		}
		models[i].Reasoning = source.Reasoning
	}
	return models
}

func (s *Service) openRouterModels() ([]Model, bool) {
	meta, ok := s.providerMeta(provider.ProviderOpenRouter)
	if !ok {
		return nil, false
	}
	return s.providerModelSnapshot(meta)
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
