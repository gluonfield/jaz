package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/onboardingstate"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	legacyOnboardingSettingsNamespace = "onboarding"
	legacyOnboardingSettingsKey       = "state"
)

type onboardingResponse struct {
	Completed       bool                       `json:"completed"`
	ACP             []onboardingACPProbe       `json:"acp"`
	NativeProviders []onboardingNativeProvider `json:"native_providers"`
	Settings        agentSettingsResponse      `json:"settings"`
}

type onboardingACPProbe struct {
	Agent                string              `json:"agent"`
	Command              string              `json:"command,omitempty"`
	Installed            bool                `json:"installed"`
	Authenticated        bool                `json:"authenticated"`
	Available            bool                `json:"available"`
	Reason               string              `json:"reason,omitempty"`
	StoragePath          string              `json:"storage_path,omitempty"`
	AuthMode             string              `json:"auth_mode,omitempty"`
	AuthPath             string              `json:"auth_path,omitempty"`
	AuthSource           string              `json:"auth_source,omitempty"`
	AuthEvidence         string              `json:"auth_evidence,omitempty"`
	RecommendedAuth      acp.AgentAuthConfig `json:"recommended_auth,omitempty"`
	AuthCommand          string              `json:"auth_command,omitempty"`
	AuthCommandAvailable bool                `json:"auth_command_available"`
	AuthCommandReason    string              `json:"auth_command_reason,omitempty"`
	RefreshOwner         string              `json:"refresh_owner,omitempty"`
}

type onboardingNativeProvider struct {
	ID         string `json:"id"`
	APIKeyEnv  string `json:"api_key_env,omitempty"`
	Configured bool   `json:"configured"`
}

type onboardingRequest struct {
	Settings     *agentsettings.AgentDefaults `json:"settings,omitempty"`
	ProviderKeys map[string]string            `json:"provider_keys,omitempty"`
	Completed    bool                         `json:"completed"`
}

func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := s.onboardingStatus(store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodPost:
		var input onboardingRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var normalized *agentsettings.AgentDefaults
		if input.Settings != nil {
			next, err := agentsettings.NormalizeAgentDefaults(*input.Settings, s.acpAgentCatalog())
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			normalized = &next
		}
		keyUpdates, err := s.providerKeyUpdates(input.ProviderKeys)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(keyUpdates) > 0 && !s.providerKeySetupAllowed(r) {
			writeError(w, http.StatusForbidden, fmt.Errorf("provider key setup is only available from the backend host"))
			return
		}
		if err := s.saveProviderKeyUpdates(keyUpdates); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if normalized != nil {
			if _, err := agentsettings.SaveAgentDefaults(store, *normalized); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		if input.Completed {
			if err := onboardingstate.Save(s.onboardingStatePath(), onboardingstate.State{Completed: true}); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		status, err := s.onboardingStatus(store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) onboardingStatus(store storage.SettingsStorage) (onboardingResponse, error) {
	state, err := s.loadOnboardingState(store)
	if err != nil {
		return onboardingResponse{}, err
	}
	defaults, err := s.loadAgentSettings(store)
	if err != nil {
		return onboardingResponse{}, err
	}
	return onboardingResponse{
		Completed:       state.Completed,
		ACP:             s.probeACPAgents(defaults),
		NativeProviders: s.nativeProviderStatuses(),
		Settings:        s.agentSettingsResponse(defaults),
	}, nil
}

func (s *Server) loadOnboardingState(store storage.SettingsStorage) (onboardingstate.State, error) {
	path := s.onboardingStatePath()
	state, found, err := onboardingstate.Load(path)
	if err != nil || found {
		return state, err
	}
	state, err = loadLegacyOnboardingState(store)
	if err != nil {
		return onboardingstate.State{}, err
	}
	if state.Completed {
		return state, onboardingstate.Save(path, state)
	}
	return state, nil
}

func loadLegacyOnboardingState(store storage.SettingsStorage) (onboardingstate.State, error) {
	setting, err := store.LoadSetting(legacyOnboardingSettingsNamespace, legacyOnboardingSettingsKey)
	if err != nil {
		if errors.Is(err, storage.ErrSettingNotFound) {
			return onboardingstate.State{}, nil
		}
		return onboardingstate.State{}, err
	}
	var state onboardingstate.State
	if err := json.Unmarshal(setting.Value, &state); err != nil {
		return onboardingstate.State{}, err
	}
	return state, nil
}

func (s *Server) onboardingStatePath() string {
	return onboardingstate.Path(s.runtimeRoot())
}

func (s *Server) probeACPAgents(defaults agentsettings.AgentDefaults) []onboardingACPProbe {
	out := []onboardingACPProbe{}
	for _, name := range s.allACPAgentNames() {
		name = acp.CanonicalAgentName(name)
		cfg, command, err := s.acpProbeConfig(name, defaults)
		if err != nil {
			out = append(out, onboardingACPProbe{Agent: name, Command: command, Reason: err.Error()})
			continue
		}
		adapterInstalled := acpCommandInstalled(cfg)
		auth := acp.ProbeAgentAuth(name, cfg, s.runtimeRoot(), nil)
		if strings.TrimSpace(cfg.URL) != "" {
			auth.Authenticated = true
			auth.Reason = ""
		}
		readiness := acp.ProbeReadiness(name, cfg, s.runtimeRoot(), nil)
		installed := adapterInstalled || auth.LoginCommandAvailable
		reason := ""
		if !installed {
			reason = firstMessage(commandMissingReason(cfg), auth.LoginCommandReason)
		} else if !adapterInstalled {
			reason = commandMissingReason(cfg)
		} else if !readiness.Available {
			reason = firstMessage(readiness.Reason, auth.LoginCommandReason, auth.Reason)
		}
		out = append(out, onboardingACPProbe{
			Agent:                name,
			Command:              command,
			Installed:            installed,
			Authenticated:        auth.Authenticated,
			Available:            readiness.Available,
			Reason:               reason,
			StoragePath:          auth.StoragePath,
			AuthMode:             auth.AuthMode,
			AuthPath:             auth.AuthPath,
			AuthSource:           auth.AuthSource,
			AuthEvidence:         auth.AuthEvidence,
			RecommendedAuth:      auth.RecommendedAuth,
			AuthCommand:          auth.LoginCommand,
			AuthCommandAvailable: auth.LoginCommandAvailable,
			AuthCommandReason:    auth.LoginCommandReason,
			RefreshOwner:         auth.RefreshOwner,
		})
	}
	return out
}

func firstMessage(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func acpCommandInstalled(cfg acp.AgentConfig) bool {
	if strings.TrimSpace(cfg.URL) != "" {
		return true
	}
	command, _ := acpCommand(cfg)
	return command != "" && acpExecutableAvailable(command)
}

func commandMissingReason(cfg acp.AgentConfig) string {
	command, _ := acpCommand(cfg)
	if strings.TrimSpace(command) == "" {
		return "command is not configured"
	}
	if _, err := acp.ResolveExecutable(command); err != nil {
		return err.Error()
	}
	return ""
}

func acpCommand(cfg acp.AgentConfig) (string, []string) {
	executable := strings.TrimSpace(cfg.Command)
	if executable == "" {
		return "", nil
	}
	return executable, cfg.Args
}

func acpExecutableAvailable(command string) bool {
	_, err := acp.ResolveExecutable(command)
	return err == nil
}

func (s *Server) acpProbeConfig(name string, defaults agentsettings.AgentDefaults) (acp.AgentConfig, string, error) {
	cfg, _ := s.acpAgentCatalog().Agent(name)
	command := strings.TrimSpace(defaults.ACP[name].Command)
	if command != "" {
		executable, args, err := agentsettings.ParseCommandLine(command)
		if err != nil {
			return acp.AgentConfig{}, command, err
		}
		cfg.Command = executable
		cfg.Args = args
	} else {
		command = agentsettings.CommandLine(cfg.Command, cfg.Args)
	}
	cfg.Model = strings.TrimSpace(defaults.ACP[name].Model)
	cfg.ReasoningEffort = strings.TrimSpace(defaults.ACP[name].ReasoningEffort)
	cfg.Auth = defaults.ACP[name].Auth
	return cfg, command, nil
}

func (s *Server) nativeProviderStatuses() []onboardingNativeProvider {
	out := []onboardingNativeProvider{}
	for _, meta := range provider.NativeProviders() {
		out = append(out, onboardingNativeProvider{
			ID:         meta.ID,
			APIKeyEnv:  meta.APIKeyEnv,
			Configured: s.providerKeyConfigured(meta.ID),
		})
	}
	return out
}

func (s *Server) providerKeyUpdates(keys map[string]string) (map[string]string, error) {
	if len(keys) == 0 {
		return nil, nil
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

func (s *Server) saveProviderKeyUpdates(updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if err := runtimeenv.Save(s.providerEnvPath(), updates); err != nil {
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
	_, ok = runtimeenv.Lookup(s.providerEnvPath(), meta.APIKeyEnv)
	return ok
}

func (s *Server) providerEnvPath() string {
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
