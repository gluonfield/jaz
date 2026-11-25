package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type acpDisconnectResponse struct {
	Settings agentsettings.ACPAgentDefaults `json:"settings"`
	Auth     acpAuthStatusResponse          `json:"auth"`
}

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
	_, release := s.claimACPAuth(r.Context(), agent)
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
	root := s.runtimeRoot()
	providers := s.modelProviders()
	if err := acp.DisconnectAgentAuth(r.Context(), agent, cfg, root, providers); err != nil {
		status := http.StatusInternalServerError
		if _, ok := errors.AsType[acp.ManualDisconnectError](err); ok {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	s.invalidateACPAuthProbe(agent)

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

	current := defaults.ACP[agent]
	cfg.Auth = current.Auth
	auth, readiness := acp.AuthAndReadinessAfterDisconnect(agent, cfg, root, providers)
	if agent == acp.AgentMuse && auth.Authenticated && auth.AuthKind != acp.AuthKindNone {
		writeError(w, http.StatusBadRequest, fmt.Errorf("Muse is signed in through META_API_KEY; remove the environment variable to disconnect"))
		return
	}

	status := s.acpAuthStatus(cfg, auth, readiness)
	release()
	writeJSON(w, http.StatusOK, acpDisconnectResponse{Settings: current, Auth: status})
}
