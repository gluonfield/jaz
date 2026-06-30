package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

var errModelCatalogUnavailable = errors.New("model provider catalog has not loaded")

var openRouterReasoningEfforts = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
	"max":     {},
}

type providerModelCatalogSnapshot struct {
	models []modelcatalog.Model
}

type modelProviderModelsResponse struct {
	Models []modelcatalog.Model `json:"models"`
}

type openRouterModelResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	ContextLength int                  `json:"context_length"`
	Pricing       openRouterPricing    `json:"pricing"`
	Reasoning     *openRouterReasoning `json:"reasoning"`
}

type openRouterPricing struct {
	Prompt          string `json:"prompt"`
	Completion      string `json:"completion"`
	InputCacheRead  string `json:"input_cache_read"`
	InputCacheWrite string `json:"input_cache_write"`
}

type openRouterReasoning struct {
	Mandatory        bool     `json:"mandatory"`
	SupportedEfforts []string `json:"supported_efforts"`
	DefaultEffort    string   `json:"default_effort"`
}

func (s *Server) handleModelProviderModels(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	models, err := s.modelProviderModels(id)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errModelCatalogUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, modelProviderModelsResponse{Models: models})
}

func (s *Server) modelProviderModels(id string) ([]modelcatalog.Model, error) {
	meta, ok := s.modelProviderMeta(id)
	if !ok {
		return nil, fmt.Errorf("unknown model provider %q", id)
	}
	switch strings.TrimSpace(meta.ID) {
	case provider.ProviderOpenAI, acp.CodexProviderOpenAIAPIKey:
		return modelcatalog.OpenAIModels(), nil
	case provider.ProviderOpenRouter:
		models, ok := s.modelCatalogSnapshot(meta)
		if !ok {
			return nil, fmt.Errorf("%w for %q", errModelCatalogUnavailable, provider.ProviderOpenRouter)
		}
		return models, nil
	default:
		if model := strings.TrimSpace(meta.DefaultModel); model != "" {
			return []modelcatalog.Model{{Value: model, Label: model}}, nil
		}
		return []modelcatalog.Model{}, nil
	}
}

func (s *Server) modelProviderMeta(id string) (provider.ModelProvider, bool) {
	if id == "" {
		return provider.ModelProvider{}, false
	}
	providers := s.modelProviders()
	key := id
	if id == acp.CodexProviderOpenAIAPIKey {
		key = provider.ProviderOpenAI
	}
	cfg := providers[key]
	meta, ok := provider.ModelProviderByID(key)
	if !ok {
		if !provider.ModelProviderConfigPresent(cfg) {
			return provider.ModelProvider{}, false
		}
		meta = provider.ModelProvider{ID: key}
	}
	meta = provider.ApplyModelProviderConfig(meta, cfg)
	if id == acp.CodexProviderOpenAIAPIKey {
		meta.ID = acp.CodexProviderOpenAIAPIKey
	}
	return meta, true
}

func (s *Server) WarmModelProviderCatalogs(ctx context.Context) error {
	meta, ok := s.modelProviderMeta(provider.ProviderOpenRouter)
	if !ok {
		return nil
	}
	key := modelCatalogKey(meta)
	s.modelCatalogMu.Lock()
	if _, ok := s.modelCatalog[key]; ok {
		s.modelCatalogMu.Unlock()
		return nil
	}
	s.modelCatalogMu.Unlock()

	models, err := fetchOpenRouterModels(ctx, meta.BaseURL)
	if err != nil {
		return err
	}

	s.setModelCatalogSnapshot(meta, models)
	return nil
}

func (s *Server) modelCatalogSnapshot(meta provider.ModelProvider) ([]modelcatalog.Model, bool) {
	key := modelCatalogKey(meta)
	s.modelCatalogMu.Lock()
	defer s.modelCatalogMu.Unlock()
	entry, ok := s.modelCatalog[key]
	if !ok {
		return nil, false
	}
	return modelcatalog.Clone(entry.models), true
}

func modelCatalogKey(meta provider.ModelProvider) string {
	return strings.TrimSpace(meta.ID) + " " + strings.TrimRight(strings.TrimSpace(meta.BaseURL), "/")
}

func (s *Server) setModelCatalogSnapshot(meta provider.ModelProvider, models []modelcatalog.Model) {
	key := modelCatalogKey(meta)
	s.modelCatalogMu.Lock()
	if s.modelCatalog == nil {
		s.modelCatalog = map[string]providerModelCatalogSnapshot{}
	}
	s.modelCatalog[key] = providerModelCatalogSnapshot{models: modelcatalog.Clone(models)}
	s.modelCatalogMu.Unlock()
}

func fetchOpenRouterModels(ctx context.Context, baseURL string) ([]modelcatalog.Model, error) {
	endpoint, err := providerModelsURL(baseURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("model provider models request failed: %d", res.StatusCode)
	}
	var body openRouterModelResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]modelcatalog.Model, 0, len(body.Data))
	for _, model := range body.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		out = append(out, modelcatalog.Model{
			Value:                  id,
			Label:                  openRouterModelLabel(model.Name, id),
			Description:            id,
			ContextLength:          model.ContextLength,
			Pricing:                parseOpenRouterPricing(model.Pricing),
			ReasoningEfforts:       cleanOpenRouterReasoningEfforts(model.Reasoning),
			ReasoningDefaultEffort: openRouterDefaultEffort(model.Reasoning),
			ReasoningMandatory:     model.Reasoning != nil && model.Reasoning.Mandatory,
		})
	}
	return out, nil
}

func providerModelsURL(baseURL string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = "https://openrouter.ai/api/v1"
	}
	u, err := url.Parse(raw + "/models")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("output_modalities", "text,image")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func openRouterModelLabel(name, fallback string) string {
	label := strings.TrimSpace(name)
	if label == "" {
		return fallback
	}
	if _, rest, ok := strings.Cut(label, ": "); ok {
		return rest
	}
	return label
}

func parseOpenRouterPricing(raw openRouterPricing) *modelcatalog.Pricing {
	input := perToken(raw.Prompt)
	output := perToken(raw.Completion)
	if input == 0 && output == 0 {
		return nil
	}
	cacheRead := perToken(raw.InputCacheRead)
	if cacheRead == 0 {
		cacheRead = input
	}
	cacheWrite := perToken(raw.InputCacheWrite)
	if cacheWrite == 0 {
		cacheWrite = input
	}
	return &modelcatalog.Pricing{
		Input:      input,
		Output:     output,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
	}
}

func perToken(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func cleanOpenRouterReasoningEfforts(reasoning *openRouterReasoning) []string {
	if reasoning == nil {
		return []string{}
	}
	out := []string{}
	for _, effort := range reasoning.SupportedEfforts {
		effort = strings.ToLower(strings.TrimSpace(effort))
		if _, ok := openRouterReasoningEfforts[effort]; ok {
			out = append(out, effort)
		}
	}
	return out
}

func openRouterDefaultEffort(reasoning *openRouterReasoning) string {
	if reasoning == nil {
		return ""
	}
	return strings.TrimSpace(reasoning.DefaultEffort)
}
