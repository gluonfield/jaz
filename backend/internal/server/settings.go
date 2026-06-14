package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type agentSettingsResponse struct {
	Native     agentsettings.NativeAgentDefaults         `json:"native"`
	Providers  []settingsNativeProvider                  `json:"providers"`
	ACP        map[string]agentsettings.ACPAgentDefaults `json:"acp"`
	ACPAuth    map[string]acpAuthStatusResponse          `json:"acp_auth"`
	ACPOptions map[string]acp.AgentOptions               `json:"acp_options"`
	Agents     []string                                  `json:"agents"`
}

// A native provider plus whether its API key is already configured on this
// backend, so Settings can show "configured" and offer to set/replace it.
type settingsNativeProvider struct {
	provider.NativeProvider
	Configured bool `json:"configured"`
}

type agentSettingsRequest struct {
	agentsettings.AgentDefaults
	ProviderKeys map[string]string `json:"provider_keys,omitempty"`
	ACPKeys      map[string]string `json:"acp_keys,omitempty"`
}

func (s *Server) handleAgentSettings(w http.ResponseWriter, r *http.Request) {
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		defaults, err := s.loadAgentSettings(store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.agentSettingsResponse(defaults))
	case http.MethodPut:
		var input agentSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		normalized, err := agentsettings.NormalizeAgentDefaults(input.AgentDefaults, s.acpAgentCatalog())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		keyUpdates, err := s.providerKeyUpdates(input.ProviderKeys)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		acpKeyUpdates, err := s.acpKeyUpdates(input.ACPKeys)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		for key, value := range acpKeyUpdates {
			keyUpdates[key] = value
		}
		if len(keyUpdates) > 0 && !s.providerKeySetupAllowed(r) {
			writeError(w, http.StatusForbidden, fmt.Errorf("agent key setup is only available from the backend host"))
			return
		}
		if err := s.saveRuntimeKeyUpdates(keyUpdates); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		saved, err := agentsettings.SaveAgentDefaults(store, normalized)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.agentSettingsResponse(saved))
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) loadAgentSettings(store storage.SettingsStorage) (agentsettings.AgentDefaults, error) {
	seed := s.agentSettingsSeed()
	defaults, err := agentsettings.LoadAgentDefaults(store)
	if err != nil {
		if !errors.Is(err, storage.ErrSettingNotFound) {
			return agentsettings.AgentDefaults{}, err
		}
		if _, err := agentsettings.SaveAgentDefaults(store, seed); err != nil {
			return agentsettings.AgentDefaults{}, err
		}
		defaults = seed
	}
	return agentsettings.MergeAgentDefaults(defaults, seed, s.allACPAgentNames()), nil
}

func (s *Server) agentSettingsResponse(defaults agentsettings.AgentDefaults) agentSettingsResponse {
	agentNames := s.allACPAgentNames()
	return agentSettingsResponse{
		Native:     defaults.Native,
		Providers:  s.nativeProvidersWithStatus(),
		ACP:        defaults.ACP,
		ACPAuth:    s.acpAgentAuthStatuses(defaults),
		ACPOptions: acpOptions(agentNames),
		Agents:     agentNames,
	}
}

func (s *Server) nativeProvidersWithStatus() []settingsNativeProvider {
	out := []settingsNativeProvider{}
	for _, meta := range provider.NativeProviders() {
		out = append(out, settingsNativeProvider{
			NativeProvider: meta,
			Configured:     s.providerKeyConfigured(meta.ID),
		})
	}
	return out
}

func (s *Server) acpAgentAuthStatuses(defaults agentsettings.AgentDefaults) map[string]acpAuthStatusResponse {
	out := make(map[string]acpAuthStatusResponse, len(s.allACPAgentNames()))
	for _, name := range s.allACPAgentNames() {
		cfg, _, err := s.acpProbeConfig(name, defaults)
		if err != nil {
			out[name] = acpAuthStatusResponse{Reason: err.Error()}
			continue
		}
		auth := acp.ProbeAgentAuth(name, cfg, s.runtimeRoot(), nil)
		out[name] = newACPAuthStatusResponse(auth)
	}
	return out
}

func acpOptions(agentNames []string) map[string]acp.AgentOptions {
	options := make(map[string]acp.AgentOptions, len(agentNames))
	for _, name := range agentNames {
		options[name] = acp.AgentOptionsFor(name)
	}
	return options
}

func (s *Server) agentSettingsSeed() agentsettings.AgentDefaults {
	return agentsettings.AgentDefaultsFromCatalog(s.acpAgentCatalog())
}

func (s *Server) allACPAgentNames() []string {
	return s.acpAgentCatalog().Names()
}

func (s *Server) acpAgentCatalog() acp.AgentCatalog {
	if len(s.AgentCatalog) > 0 {
		return s.AgentCatalog
	}
	return acp.BuiltinAgents()
}
