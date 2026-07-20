package acp

import (
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

func modelProviderSecretEnvNames(configs map[string]provider.ModelProviderConfig) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, modelProvider := range provider.ModelProviders() {
		if key := strings.TrimSpace(modelProvider.APIKeyEnv); key != "" {
			keys[key] = struct{}{}
			if alias := apiKeyAlias(key); alias != "" {
				keys[alias] = struct{}{}
			}
		}
	}
	for id, cfg := range configs {
		if key := provider.ConfiguredAPIKeyEnv(id, cfg); key != "" {
			keys[key] = struct{}{}
			if alias := apiKeyAlias(key); alias != "" {
				keys[alias] = struct{}{}
			}
		}
	}
	return keys
}
