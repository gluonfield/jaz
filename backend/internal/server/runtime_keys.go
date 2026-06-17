package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
)

// modelProviders returns a snapshot of the current effective provider set
// (catalog + application.yaml + DB customs).
func (s *Server) modelProviders() map[string]provider.ModelProviderConfig {
	if s.Providers == nil {
		return map[string]provider.ModelProviderConfig{}
	}
	return s.Providers.Providers()
}

// reloadProviders rebuilds the live provider registry (so ACP spawns and settings
// reads see DB changes) and the native provider clients (so native key changes
// apply). Call after any provider create/update/delete or key change.
func (s *Server) reloadProviders() error {
	if s.Providers != nil {
		if err := s.Providers.Reload(); err != nil {
			return err
		}
	}
	if s.NativeProviders != nil {
		return s.NativeProviders.Reload()
	}
	return nil
}

func (s *Server) providerKeyUpdates(keys map[string]string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	providers := s.modelProviders()
	updates := map[string]string{}
	for id, key := range keys {
		id = strings.TrimSpace(id)
		cfg := providers[id]
		meta, ok := provider.ModelProviderByID(id)
		if !ok {
			if !provider.ModelProviderConfigPresent(cfg) {
				return nil, fmt.Errorf("unknown model provider %q", id)
			}
			meta = provider.ModelProvider{ID: id}
		}
		meta = provider.ApplyModelProviderConfig(meta, cfg)
		keyEnv := strings.TrimSpace(meta.APIKeyEnv)
		if keyEnv == "" {
			return nil, fmt.Errorf("model provider %q has no API key env var", id)
		}
		if strings.TrimSpace(key) != "" {
			updates[keyEnv] = key
		}
	}
	return updates, nil
}

func (s *Server) acpKeyUpdates(keys map[string]string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	allowed := map[string]struct{}{}
	for _, name := range s.allACPAgentNames() {
		name = acp.CanonicalAgentName(name)
		if _, ok := acp.AgentAPIKey(name); ok {
			allowed[name] = struct{}{}
		}
	}
	updates := map[string]string{}
	for name, key := range keys {
		name = acp.CanonicalAgentName(name)
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("unknown acp agent %q", name)
		}
		spec, _ := acp.AgentAPIKey(name)
		if strings.TrimSpace(key) != "" {
			updates[spec.SourceEnv] = key
		}
	}
	return updates, nil
}

// applyRuntimeKeyUpdates collects native provider + ACP agent key updates from a
// request, enforces the host-only guard, and persists them. Returns the HTTP
// status to use on failure (0 on success) so handlers stay one line. Shared by
// the settings and onboarding endpoints.
func (s *Server) applyRuntimeKeyUpdates(r *http.Request, providerKeys, acpKeys map[string]string) (int, error) {
	keyUpdates, err := s.providerKeyUpdates(providerKeys)
	if err != nil {
		return http.StatusBadRequest, err
	}
	acpKeyUpdates, err := s.acpKeyUpdates(acpKeys)
	if err != nil {
		return http.StatusBadRequest, err
	}
	for key, value := range acpKeyUpdates {
		keyUpdates[key] = value
	}
	if len(keyUpdates) > 0 && !s.providerKeySetupAllowed(r) {
		return http.StatusForbidden, fmt.Errorf("key setup is only available from the backend host")
	}
	if err := s.saveRuntimeKeyUpdates(keyUpdates); err != nil {
		return http.StatusBadRequest, err
	}
	return 0, nil
}

func (s *Server) saveRuntimeKeyUpdates(updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if err := runtimeenv.Save(s.runtimeKeyEnvPath(), updates); err != nil {
		return err
	}
	// Reload both the native clients and the provider registry so a key set here
	// reaches native turns and the next opencode spawn without a restart.
	return s.reloadProviders()
}

func (s *Server) providerKeySetupAllowed(r *http.Request) bool {
	return strings.TrimSpace(s.AuthKey) != "" || loopbackRequest(r)
}

func (s *Server) providerKeyConfigured(id string) bool {
	if s.NativeProviders != nil {
		return s.NativeProviders.APIKeyConfigured(id)
	}
	meta, ok := provider.NativeProviderByID(id)
	if !ok || strings.TrimSpace(meta.APIKeyEnv) == "" {
		return false
	}
	if strings.TrimSpace(os.Getenv(meta.APIKeyEnv)) != "" {
		return true
	}
	_, ok = runtimeenv.Lookup(s.runtimeKeyEnvPath(), meta.APIKeyEnv)
	return ok
}

func (s *Server) validateNativeProviderRunnable(id string) error {
	if s.NativeProviders == nil {
		return nil
	}
	meta, ok := provider.NativeProviderByID(id)
	if !ok {
		return fmt.Errorf("unknown native provider %q", id)
	}
	if strings.TrimSpace(meta.APIKeyEnv) == "" || s.providerKeyConfigured(id) {
		return nil
	}
	return fmt.Errorf("native provider %q cannot run without %s; add the key in Settings > Agents", id, meta.APIKeyEnv)
}

func (s *Server) runtimeKeyEnvPath() string {
	if s.NativeProviders != nil {
		if path := strings.TrimSpace(s.NativeProviders.APIKeyEnvPath()); path != "" {
			return path
		}
	}
	return runtimeenv.Path(s.runtimeRoot())
}

func (s *Server) runtimeRoot() string {
	if strings.TrimSpace(s.Root) != "" {
		return s.Root
	}
	if rooter, ok := s.Store.(interface{ RootDir() string }); ok {
		return rooter.RootDir()
	}
	return "."
}
