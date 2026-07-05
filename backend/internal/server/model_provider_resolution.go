package server

import (
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

type resolvedModelProvider struct {
	ID     string
	Config provider.ModelProviderConfig
	Meta   provider.ModelProvider
	Custom bool
}

func (s *Server) resolvedModelProviders() []resolvedModelProvider {
	providers := s.modelProviders()
	custom := s.customProviderIDSet()
	out := make([]resolvedModelProvider, 0, len(providers)+len(provider.ModelProviders()))
	seen := map[string]struct{}{}
	for _, meta := range provider.ModelProviders() {
		cfg := providers[meta.ID]
		meta = provider.ApplyModelProviderConfig(meta, cfg)
		out = append(out, resolvedModelProvider{ID: meta.ID, Config: cfg, Meta: meta})
		seen[meta.ID] = struct{}{}
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		cfg := providers[id]
		meta := provider.ApplyModelProviderConfig(provider.ModelProvider{ID: id}, cfg)
		out = append(out, resolvedModelProvider{ID: id, Config: cfg, Meta: meta, Custom: custom[id]})
	}
	return out
}

func (s *Server) resolvedModelProvider(id string) (resolvedModelProvider, bool) {
	for _, modelProvider := range s.resolvedModelProviders() {
		if modelProvider.ID == id {
			return modelProvider, true
		}
	}
	return resolvedModelProvider{}, false
}

func (s *Server) modelProviderConfigReady(id string, cfg provider.ModelProviderConfig, meta provider.ModelProvider) bool {
	if meta.RequiresAPIKey {
		return s.modelProviderKeyConfigured(id, cfg, meta)
	}
	if meta.OpenAICompatible || strings.EqualFold(strings.TrimSpace(cfg.Type), "openai-compatible") {
		return strings.TrimSpace(meta.BaseURL) != ""
	}
	return true
}
