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
	Native    agentsettings.NativeAgentDefaults         `json:"native"`
	Providers []provider.NativeProvider                 `json:"providers"`
	ACP       map[string]agentsettings.ACPAgentDefaults `json:"acp"`
	Agents    []string                                  `json:"agents"`
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
		var input agentsettings.AgentDefaults
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		normalized, err := agentsettings.NormalizeAgentDefaults(input, s.acpAgentCatalog())
		if err != nil {
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
	return agentSettingsResponse{
		Native:    defaults.Native,
		Providers: provider.NativeProviders(),
		ACP:       defaults.ACP,
		Agents:    s.allACPAgentNames(),
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
