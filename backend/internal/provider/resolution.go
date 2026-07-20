package provider

import (
	"sort"
	"strings"
)

type ResolvedModelProvider struct {
	ID         string
	Config     ModelProviderConfig
	Meta       ModelProvider
	BuiltIn    bool
	Configured bool
}

func ResolveModelProvider(id string, configs map[string]ModelProviderConfig) ResolvedModelProvider {
	id = strings.ToLower(strings.TrimSpace(id))
	cfg, configured := configs[id]
	meta, builtIn := ModelProviderByID(id)
	if !builtIn {
		meta = ModelProvider{ID: id}
	}
	if configured {
		meta = ApplyModelProviderConfig(meta, cfg)
	}
	return ResolvedModelProvider{ID: id, Config: cfg, Meta: meta, BuiltIn: builtIn, Configured: configured}
}

func ResolveModelProviders(configs map[string]ModelProviderConfig) []ResolvedModelProvider {
	providers := ModelProviders()
	out := make([]ResolvedModelProvider, 0, len(providers)+len(configs))
	seen := make(map[string]struct{}, len(providers))
	for _, meta := range providers {
		resolved := ResolveModelProvider(meta.ID, configs)
		out = append(out, resolved)
		seen[resolved.ID] = struct{}{}
	}
	ids := make([]string, 0, len(configs))
	for id := range configs {
		id = strings.ToLower(strings.TrimSpace(id))
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, ResolveModelProvider(id, configs))
	}
	return out
}
