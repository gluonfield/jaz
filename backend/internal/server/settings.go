package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type agentSettingsResponse struct {
	Providers  []settingsModelProvider                   `json:"providers"`
	ACP        map[string]agentsettings.ACPAgentDefaults `json:"acp"`
	ACPAuth    map[string]acpAuthStatusResponse          `json:"acp_auth"`
	ACPOptions map[string]acp.AgentOptions               `json:"acp_options"`
	Agents     []string                                  `json:"agents"`
}

type settingsModelProvider struct {
	provider.ModelProvider
	Configured bool `json:"configured"`
	// Custom marks a user-created (DB-backed) provider, editable/deletable in the
	// UI. Built-ins and application.yaml providers are not custom.
	Custom bool `json:"custom,omitempty"`
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
		if status, err := s.applyRuntimeKeyUpdates(r, input.ProviderKeys, input.ACPKeys); err != nil {
			writeError(w, status, err)
			return
		}
		if err := s.validateEnabledACPAgentAuth(normalized); err != nil {
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
	providers := s.modelProvidersWithStatus()
	return agentSettingsResponse{
		Providers:  providers,
		ACP:        defaults.ACP,
		ACPAuth:    s.acpAgentAuthStatuses(defaults),
		ACPOptions: acpOptions(s.acpAgentCatalog(), agentNames, providers),
		Agents:     agentNames,
	}
}

func (s *Server) modelProvidersWithStatus() []settingsModelProvider {
	providers := s.modelProviders()
	custom := s.customProviderIDSet()
	out := []settingsModelProvider{}
	seen := map[string]struct{}{}
	for _, meta := range provider.ModelProviders() {
		cfg := providers[meta.ID]
		meta = provider.ApplyModelProviderConfig(meta, cfg)
		out = append(out, settingsModelProvider{
			ModelProvider: meta,
			Configured:    s.modelProviderKeyConfigured(meta.ID, cfg, meta),
		})
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
		out = append(out, settingsModelProvider{
			ModelProvider: meta,
			Configured:    s.modelProviderKeyConfigured(id, cfg, meta),
			Custom:        custom[id],
		})
	}
	return out
}

func (s *Server) modelProviderKeyConfigured(id string, cfg provider.ModelProviderConfig, meta provider.ModelProvider) bool {
	if s.providerKeyConfigured(id) {
		return true
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	keyEnv := firstNonEmpty(cfg.APIKeyEnv, meta.APIKeyEnv)
	if keyEnv == "" {
		return false
	}
	if strings.TrimSpace(os.Getenv(keyEnv)) != "" {
		return true
	}
	_, ok := runtimeenv.Lookup(s.runtimeKeyEnvPath(), keyEnv)
	return ok
}

func (s *Server) modelProviderConfigured(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, provider := range s.modelProvidersWithStatus() {
		if provider.ID == id {
			return provider.Implemented && provider.Configured
		}
	}
	return false
}

func (s *Server) acpAgentAuthStatuses(defaults agentsettings.AgentDefaults) map[string]acpAuthStatusResponse {
	out := make(map[string]acpAuthStatusResponse, len(s.allACPAgentNames()))
	for _, name := range s.allACPAgentNames() {
		cfg, _, err := s.acpProbeConfig(name, defaults)
		if err != nil {
			out[name] = acpAuthStatusResponse{Reason: err.Error()}
			continue
		}
		auth := acp.ProbeAgentAuthWithProviders(name, cfg, s.runtimeRoot(), nil, s.modelProviders())
		out[name] = newACPAuthStatusResponse(auth)
	}
	return out
}

func acpOptions(catalog acp.AgentCatalog, agentNames []string, providers []settingsModelProvider) map[string]acp.AgentOptions {
	options := make(map[string]acp.AgentOptions, len(agentNames))
	for _, name := range agentNames {
		cfg, _ := catalog.Agent(name)
		option := acp.AgentOptionsForConfig(name, cfg)
		if cfg.UsesModelProvider() {
			option.ModelProviderIDs = compatibleModelProviderIDs(cfg.ModelProviderCapability, providers)
		}
		options[name] = option
	}
	return options
}

func compatibleModelProviderIDs(capability string, providers []settingsModelProvider) []string {
	ids := []string{}
	for _, modelProvider := range providers {
		if modelProvider.SupportsCapability(capability) {
			ids = append(ids, modelProvider.ID)
		}
	}
	return ids
}

func (s *Server) validateEnabledACPAgentAuth(defaults agentsettings.AgentDefaults) error {
	for _, name := range s.allACPAgentNames() {
		name = acp.CanonicalAgentName(name)
		current, ok := defaults.ACP[name]
		if !ok || !current.Enabled {
			continue
		}
		cfg, _, err := s.acpProbeConfig(name, defaults)
		if err != nil {
			return err
		}
		if !enabledACPAgentRequiresAuth(name, cfg) {
			continue
		}
		auth := acp.ProbeAgentAuthWithProviders(name, cfg, s.runtimeRoot(), nil, s.modelProviders())
		if !auth.Authenticated {
			reason := firstMessage(auth.Reason, "connect the agent or add an API key")
			return fmt.Errorf("acp agent %q cannot be enabled without authentication: %s", name, reason)
		}
	}
	return nil
}

func enabledACPAgentRequiresAuth(name string, cfg acp.AgentConfig) bool {
	if cfg.Local || strings.TrimSpace(cfg.URL) != "" {
		return false
	}
	switch acp.CanonicalAgentName(name) {
	case acp.AgentCodex, acp.AgentClaude, acp.AgentGrok, acp.AgentOpenCode:
		return true
	default:
		return false
	}
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
