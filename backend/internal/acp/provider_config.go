package acp

import (
	"strings"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

type modelProviderResolution struct {
	meta       modelprovider.ModelProvider
	config     modelprovider.ModelProviderConfig
	builtIn    bool
	configured bool
}

func resolveModelProvider(id string, providers map[string]modelprovider.ModelProviderConfig) modelProviderResolution {
	id = strings.ToLower(strings.TrimSpace(id))
	cfg, configured := providers[id]
	meta, builtIn := modelprovider.ModelProviderByID(id)
	if !builtIn {
		meta = modelprovider.ModelProvider{ID: id}
	}
	if configured {
		meta = modelprovider.ApplyModelProviderConfig(meta, cfg)
	}
	return modelProviderResolution{meta: meta, config: cfg, builtIn: builtIn, configured: configured}
}
