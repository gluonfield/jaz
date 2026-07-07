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

// handleDisconnectACPAuth removes an agent's credential so it can be re-connected
// or switched. It deletes only what Jaz owns: the Jaz-managed API key env var,
// and an OAuth credential that lives in Jaz's own profile (or Grok's and
// Antigravity's CLI-owned logins, which have no other sign-out). It never
// touches the user's global ~/.claude.json / ~/.codex config.
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
	auth := acp.ProbeAgentAuthWithProviders(agent, cfg, s.runtimeRoot(), nil, s.modelProviders())

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

	if err := acp.RemoveOwnedCredential(agent, auth.StoragePath, s.runtimeRoot()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Antigravity's login lives in CLI-owned stores Jaz can only best-effort
	// clear; fail loudly instead of showing a still-connected card if the agy
	// CLI kept its credentials (e.g. it moved its keyring entry).
	if agent == acp.AgentAntigravity && auth.Authenticated {
		if after := acp.ProbeAgentAuthWithProviders(agent, cfg, s.runtimeRoot(), nil, s.modelProviders()); after.Authenticated {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("antigravity is still signed in after removing its credentials; sign out with the agy CLI and reconnect"))
			return
		}
	}

	writeJSON(w, http.StatusOK, s.agentSettingsResponse(defaults))
}
