package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) handleDisconnectACPAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	agent := acp.CanonicalAgentName(r.PathValue("agent"))
	if _, ok := s.acpAgentCatalog().Agent(agent); !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown acp agent %q", agent))
		return
	}
	if !s.providerKeySetupAllowed(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("disconnecting is only available from the backend host"))
		return
	}
	release, err := s.claimACPAuth(agent)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	defaults, err := s.loadAgentSettings(store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg, _, err := s.acpProbeConfig(agent, defaults)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var storagePath string
	if agent != acp.AgentAntigravity {
		auth := acp.ProbeAgentAuthWithProviders(agent, cfg, s.runtimeRoot(), nil, s.modelProviders())
		if auth.ManualDisconnect != "" {
			writeError(w, http.StatusConflict, fmt.Errorf("%s", auth.ManualDisconnect))
			return
		}
		storagePath = auth.StoragePath
	}
	if err := acp.DisconnectAgentAuth(r.Context(), agent, cfg, s.runtimeRoot(), storagePath); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if current, ok := defaults.ACP[agent]; ok {
		current.Enabled = false
		current.Auth = acp.DisconnectedAuthConfig(agent, current.Auth)
		defaults.ACP[agent] = current
		if defaults, err = agentsettings.SaveAgentDefaults(store, defaults); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	if spec, ok := acp.AgentAPIKey(agent); ok && strings.TrimSpace(spec.SourceEnv) != "" {
		if err := runtimeenv.Remove(s.runtimeKeyEnvPath(), spec.SourceEnv); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	release()
	response := s.agentSettingsResponse(defaults)
	if agent == acp.AgentMuse && response.ACPAuth[agent].Authenticated && response.ACPAuth[agent].AuthKind != acp.AuthKindNone {
		writeError(w, http.StatusBadRequest, fmt.Errorf("Muse is signed in through META_API_KEY; remove the environment variable to disconnect"))
		return
	}

	writeJSON(w, http.StatusOK, response)
}
