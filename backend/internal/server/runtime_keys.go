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

func (s *Server) providerKeyUpdates(keys map[string]string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	updates := map[string]string{}
	for id, key := range keys {
		meta, ok := provider.NativeProviderByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown native provider %q", id)
		}
		if strings.TrimSpace(meta.APIKeyEnv) == "" {
			return nil, fmt.Errorf("native provider %q has no API key env var", id)
		}
		if strings.TrimSpace(key) != "" {
			updates[meta.APIKeyEnv] = key
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

func (s *Server) saveRuntimeKeyUpdates(updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if err := runtimeenv.Save(s.runtimeKeyEnvPath(), updates); err != nil {
		return err
	}
	if s.NativeProviders != nil {
		return s.NativeProviders.Reload()
	}
	return nil
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
