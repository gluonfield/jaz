package provider

import "sync"

// CustomProviderLoader supplies user-defined provider configs (DB-backed) to
// overlay on top of the static catalog + application.yaml base. Kept as an
// interface so the provider package has no storage dependency.
type CustomProviderLoader interface {
	CustomProviderConfigs() (map[string]ModelProviderConfig, error)
}

// Source is the single source of truth for the current effective provider set —
// the built-in catalog + application.yaml base, overlaid with DB-backed customs.
// Every runtime consumer (native runtime, ACP spawn, settings/auth probes) reads
// through it, so a runtime add/edit/delete propagates without a restart.
// Providers() returns an owned copy so callers (e.g. ACP spawn goroutines) can
// range it freely without racing a concurrent Reload().
type Source interface {
	Providers() map[string]ModelProviderConfig
	Reload() error
}

type mergedSource struct {
	base   map[string]ModelProviderConfig
	loader CustomProviderLoader
	mu     sync.RWMutex
	merged map[string]ModelProviderConfig
}

// NewSource builds a Source over the immutable base config plus a custom-provider
// loader, performing an initial Reload so it's usable immediately.
func NewSource(base map[string]ModelProviderConfig, loader CustomProviderLoader) (Source, error) {
	s := &mergedSource{base: cloneProviderConfigs(base), loader: loader}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *mergedSource) Reload() error {
	merged := cloneProviderConfigs(s.base)
	if s.loader != nil {
		customs, err := s.loader.CustomProviderConfigs()
		if err != nil {
			return err
		}
		for id, cfg := range customs {
			// Built-ins always win: a custom can never shadow or remove a base
			// provider. (providerstore also refuses reserved ids at write time.)
			if _, ok := merged[id]; ok {
				continue
			}
			merged[id] = cfg
		}
	}
	s.mu.Lock()
	s.merged = merged
	s.mu.Unlock()
	return nil
}

func (s *mergedSource) Providers() map[string]ModelProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProviderConfigs(s.merged)
}

// StaticSource returns a Source over a fixed snapshot; Reload is a no-op. Used by
// the read-time auth/readiness probes, which already work from a plain map.
func StaticSource(providers map[string]ModelProviderConfig) Source {
	return staticSource{providers: cloneProviderConfigs(providers)}
}

type staticSource struct {
	providers map[string]ModelProviderConfig
}

func (s staticSource) Providers() map[string]ModelProviderConfig {
	return cloneProviderConfigs(s.providers)
}

func (staticSource) Reload() error { return nil }

func cloneProviderConfigs(in map[string]ModelProviderConfig) map[string]ModelProviderConfig {
	out := make(map[string]ModelProviderConfig, len(in))
	for id, cfg := range in {
		cfg.Capabilities = append([]string(nil), cfg.Capabilities...)
		out[id] = cfg
	}
	return out
}
