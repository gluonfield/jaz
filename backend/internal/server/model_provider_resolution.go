package server

import (
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

type resolvedModelProvider struct {
	provider.ResolvedModelProvider
	Custom bool
}

func (s *Server) resolvedModelProviders() []resolvedModelProvider {
	providers := s.modelProviders()
	custom := s.customProviderIDSet()
	resolved := provider.ResolveModelProviders(providers)
	out := make([]resolvedModelProvider, 0, len(resolved))
	for _, modelProvider := range resolved {
		out = append(out, resolvedModelProvider{ResolvedModelProvider: modelProvider, Custom: custom[modelProvider.ID]})
	}
	return out
}

func (s *Server) resolvedModelProvider(id string) (resolvedModelProvider, bool) {
	resolved := provider.ResolveModelProvider(id, s.modelProviders())
	if !resolved.BuiltIn && !resolved.Configured {
		return resolvedModelProvider{}, false
	}
	return resolvedModelProvider{ResolvedModelProvider: resolved, Custom: s.customProviderIDSet()[resolved.ID]}, true
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
