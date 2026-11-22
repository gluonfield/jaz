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
	providers    ProviderSource
	apiKeyLookup func(string) string

	warmMu         sync.Mutex
	mu             sync.RWMutex
	providerModels map[string][]Model
}

func NewService(providers ProviderSource) *Service {
	return &Service{providers: providers}
}

func NewServiceWithAPIKeyLookup(providers ProviderSource, lookup func(string) string) *Service {
	return &Service{providers: providers, apiKeyLookup: lookup}
}

func (s *Service) Warm(ctx context.Context) error {
	resolved, ok := s.resolvedProvider(provider.ProviderOpenRouter)
	if !ok {
		return nil
	}
	meta := resolved.Meta
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

// catalogURL picks where the provider's model list lives. Some inferences
// (e.g. OpenCode Go) serve /models on a sibling path, so a provider may
// override the default base-url + "/models".
func catalogURL(meta provider.ModelProvider, baseURL string, modelsURL string) string {
	if u := strings.TrimSpace(modelsURL); u != "" {
		return u
	}
	_ = meta
	return ""
}

func (s *Service) ProviderModels(id string) ([]Model, error) {
	resolved, ok := s.resolvedProvider(strings.ToLower(strings.TrimSpace(id)))
	if !ok {
		return nil, fmt.Errorf("unknown model provider %q", id)
	}
	meta := resolved.Meta
	switch strings.TrimSpace(meta.ID) {
	case provider.ProviderOpenAI, codexOpenAIAPIKeyProvider:
		return s.enrichReasoning(cloneModels(openAIModels)), nil
	case provider.ProviderOpenRouter:
		models, ok := s.providerModelSnapshot(meta)
		if !ok {
			return nil, fmt.Errorf("%w for %q", ErrCatalogUnavailable, provider.ProviderOpenRouter)
		}
		return models, nil
	case provider.ProviderOllama:
		models, err := fetchOllamaModels(context.Background(), meta.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("%w for %q: %w", ErrCatalogUnavailable, meta.ID, err)
		}
		return models, nil
	default:
		if len(resolved.Config.Models) > 0 {
			models := make([]Model, 0, len(resolved.Config.Models))
			for _, name := range resolved.Config.Models {
				if name = strings.TrimSpace(name); name != "" {
					models = append(models, completeModelMetadata(modelWithoutProviderReasoning(name, name, "")))
				}
			}
			return models, nil
		}
		cfg := resolved.Config
		apiKey := cfg.APIKey
		if strings.TrimSpace(apiKey) == "" && s.apiKeyLookup != nil && meta.APIKeyEnv != "" {
			apiKey = s.apiKeyLookup(meta.APIKeyEnv)
		}
		if meta.OpenAICompatible && (!meta.RequiresAPIKey || apiKey != "") {
			models, err := fetchOpenAICompatibleModels(context.Background(), catalogEndpoint(meta.BaseURL, cfg.ModelsURL, cfg.BaseURL), apiKey, cfg.Headers)
			if err == nil && len(models) > 0 {
				return models, nil
			}
			if strings.TrimSpace(meta.DefaultModel) == "" && err != nil {
				return nil, fmt.Errorf("%w for %q: %w", ErrCatalogUnavailable, meta.ID, err)
			}
		}
		if model := strings.TrimSpace(meta.DefaultModel); model != "" {
			return []Model{completeModelMetadata(modelWithoutProviderReasoning(model, model, ""))}, nil
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
	resolved, ok := s.resolvedProvider(provider.ProviderOpenRouter)
	if !ok {
		return nil, false
	}
	return s.providerModelSnapshot(resolved.Meta)
}

func (s *Service) resolvedProvider(id string) (provider.ResolvedModelProvider, bool) {
	if id == "" {
		return provider.ResolvedModelProvider{}, false
	}
	key := id
	if id == codexOpenAIAPIKeyProvider {
		key = provider.ProviderOpenAI
	}
	var cfg map[string]provider.ModelProviderConfig
	if s.providers != nil {
		cfg = s.providers.Providers()
	}
	resolved := provider.ResolveModelProvider(key, cfg)
	if !resolved.BuiltIn && !provider.ModelProviderConfigPresent(resolved.Config) {
		return provider.ResolvedModelProvider{}, false
	}
	if id == codexOpenAIAPIKeyProvider {
		if !resolved.Meta.SupportsCapability(provider.CapabilityResponses) {
			return provider.ResolvedModelProvider{}, false
		}
		resolved.Meta.ID = codexOpenAIAPIKeyProvider
	}
	return resolved, true
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
