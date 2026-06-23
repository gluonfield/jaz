package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpadapter"
	"github.com/wins/jaz/backend/internal/onboardingstate"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type onboardingResponse struct {
	Completed bool                         `json:"completed"`
	ACP       []onboardingACPProbe         `json:"acp"`
	Settings  agentSettingsResponse        `json:"settings"`
	Memory    agentsettings.MemorySettings `json:"memory"`
}

type onboardingACPProbe struct {
	acpAuthStatusResponse
	Agent                string                      `json:"agent"`
	Command              string                      `json:"command,omitempty"`
	Installed            bool                        `json:"installed"`
	AppInstalled         bool                        `json:"app_installed,omitempty"`
	AppName              string                      `json:"app_name,omitempty"`
	Available            bool                        `json:"available"`
	AuthCommand          string                      `json:"auth_command,omitempty"`
	AuthCommandAvailable bool                        `json:"auth_command_available"`
	AuthCommandReason    string                      `json:"auth_command_reason,omitempty"`
	ManagedAdapter       *onboardingACPAdapterStatus `json:"managed_adapter,omitempty"`
}

type onboardingACPAdapterStatus struct {
	Adapter  string `json:"adapter"`
	Version  string `json:"version,omitempty"`
	Platform string `json:"platform,omitempty"`
	State    string `json:"state"`
	Message  string `json:"message,omitempty"`
}

type onboardingRequest struct {
	Settings     *agentsettings.AgentDefaults  `json:"settings,omitempty"`
	Memory       *agentsettings.MemorySettings `json:"memory,omitempty"`
	ProviderKeys map[string]string             `json:"provider_keys,omitempty"`
	ACPKeys      map[string]string             `json:"acp_keys,omitempty"`
	Completed    bool                          `json:"completed"`
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
		if status, err := s.applyRuntimeKeyUpdates(r, input.ProviderKeys, input.ACPKeys); err != nil {
			writeError(w, status, err)
			return
		}
		var defaults agentsettings.AgentDefaults
		if normalized != nil {
			if err := s.validateEnabledACPAgentAuth(*normalized); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			saved, err := agentsettings.SaveAgentDefaults(store, *normalized)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			defaults = saved
		}
		if input.Memory != nil || input.Completed {
			if defaults.ACP == nil {
				var err error
				defaults, err = s.loadAgentSettings(store)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
			}
			if input.Completed && normalized == nil {
				if err := s.validateEnabledACPAgentAuth(defaults); err != nil {
					writeError(w, http.StatusBadRequest, err)
					return
				}
			}
			if status, err := saveOnboardingMemorySettings(store, defaults, input.Memory); err != nil {
				writeError(w, status, err)
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

func saveOnboardingMemorySettings(store storage.SettingsStorage, defaults agentsettings.AgentDefaults, input *agentsettings.MemorySettings) (int, error) {
	settings, status, err := onboardingMemorySettings(store, defaults, input)
	if err != nil {
		return status, err
	}
	if _, err := agentsettings.SaveMemorySettings(store, settings); err != nil {
		return http.StatusInternalServerError, err
	}
	return 0, nil
}

func onboardingMemorySettings(store storage.SettingsStorage, defaults agentsettings.AgentDefaults, input *agentsettings.MemorySettings) (agentsettings.MemorySettings, int, error) {
	if input != nil {
		settings, err := normalizeOnboardingMemorySettings(defaults, *input)
		if err != nil {
			return agentsettings.MemorySettings{}, http.StatusBadRequest, err
		}
		return settings, 0, nil
	}
	settings, err := agentsettings.LoadMemorySettings(store)
	if err != nil {
		return agentsettings.MemorySettings{}, http.StatusInternalServerError, err
	}
	if strings.TrimSpace(settings.Agent) == "" {
		settings.Agent = agentsettings.DefaultMemoryAgent(defaults)
	}
	settings, err = normalizeOnboardingMemorySettings(defaults, settings)
	if err != nil {
		return agentsettings.MemorySettings{}, http.StatusBadRequest, err
	}
	return settings, 0, nil
}

func normalizeOnboardingMemorySettings(defaults agentsettings.AgentDefaults, settings agentsettings.MemorySettings) (agentsettings.MemorySettings, error) {
	settings.Agent = acp.CanonicalAgentName(settings.Agent)
	if settings.Enabled && strings.TrimSpace(settings.Agent) == "" {
		return agentsettings.MemorySettings{}, fmt.Errorf("memory agent is required when memory is enabled")
	}
	if settings.Agent == "" {
		return settings, nil
	}
	if settings.Agent == acp.AgentJaz {
		return agentsettings.MemorySettings{}, fmt.Errorf("built-in Jaz cannot be used as the memory agent yet")
	}
	if err := validateMemoryAgent(defaults, settings.Agent); err != nil {
		return agentsettings.MemorySettings{}, err
	}
	return settings, nil
}

func (s *Server) onboardingStatus(store storage.SettingsStorage) (onboardingResponse, error) {
	state, _, err := onboardingstate.Load(s.onboardingStatePath())
	if err != nil {
		return onboardingResponse{}, err
	}
	defaults, err := s.loadAgentSettings(store)
	if err != nil {
		return onboardingResponse{}, err
	}
	memory, err := agentsettings.LoadMemorySettings(store)
	if err != nil {
		return onboardingResponse{}, err
	}
	return onboardingResponse{
		Completed: state.Completed,
		ACP:       s.probeACPAgents(defaults),
		Settings:  s.agentSettingsResponse(defaults),
		Memory:    memory,
	}, nil
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
			out = append(out, onboardingACPProbe{
				acpAuthStatusResponse: acpAuthStatusResponse{Reason: err.Error()},
				Agent:                 name,
				Command:               command,
			})
			continue
		}
		adapterInstalled := acpCommandInstalled(cfg)
		auth := acp.ProbeAgentAuthWithProviders(name, cfg, s.runtimeRoot(), nil, s.modelProviders())
		if strings.TrimSpace(cfg.URL) != "" {
			auth.Authenticated = true
			auth.Reason = ""
		}
		appName, appInstalled := agentAppInstall(name)
		readiness := acp.ProbeReadinessWithProviders(name, cfg, s.runtimeRoot(), nil, s.modelProviders())
		adapter := s.managedAdapterStatus(cfg)
		adapterReady := adapter == nil || adapter.State == acpadapter.StateReady
		installed := adapterInstalled || auth.LoginCommandAvailable
		reason := ""
		if !installed {
			reason = firstMessage(commandMissingReason(cfg), auth.LoginCommandReason)
		} else if !adapterInstalled {
			reason = commandMissingReason(cfg)
		} else if !adapterReady {
			reason = adapter.Message
		} else if !readiness.Available {
			reason = firstMessage(readiness.Reason, auth.LoginCommandReason, auth.Reason)
		}
		authResponse := newACPAuthStatusResponse(auth)
		authResponse.Reason = reason
		out = append(out, onboardingACPProbe{
			acpAuthStatusResponse: authResponse,
			Agent:                 name,
			Command:               command,
			Installed:             installed,
			AppInstalled:          appInstalled,
			AppName:               appName,
			Available:             readiness.Available && adapterReady,
			AuthCommand:           auth.LoginCommand,
			AuthCommandAvailable:  auth.LoginCommandAvailable,
			AuthCommandReason:     auth.LoginCommandReason,
			ManagedAdapter:        adapter,
		})
	}
	return out
}

func (s *Server) managedAdapterStatus(cfg acp.AgentConfig) *onboardingACPAdapterStatus {
	adapter := strings.TrimSpace(cfg.ManagedAdapter)
	if adapter == "" {
		return nil
	}
	status := acpadapter.Status{
		Adapter: adapter,
		State:   acpadapter.StateMissing,
		Message: "managed adapter downloader is not available",
	}
	if s.ACPAdapters != nil {
		status = s.ACPAdapters.Status(adapter)
	}
	return &onboardingACPAdapterStatus{
		Adapter:  status.Adapter,
		Version:  status.Version,
		Platform: status.Platform,
		State:    status.State,
		Message:  status.Message,
	}
}

func agentAppInstall(name string) (string, bool) {
	if name == acp.AgentClaude && appBundleInstalled("Claude.app") {
		return "Claude app", true
	}
	return "", false
}

func appBundleInstalled(bundle string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return appBundleInstalledIn(dirs, bundle)
}

func appBundleInstalledIn(dirs []string, bundle string) bool {
	for _, dir := range dirs {
		info, err := os.Stat(filepath.Join(dir, bundle))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
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
	if !cfg.RequiresCommand() {
		return true
	}
	if strings.TrimSpace(cfg.URL) != "" {
		return true
	}
	command, _ := acpCommand(cfg)
	return command != "" && acpExecutableAvailable(command)
}

func commandMissingReason(cfg acp.AgentConfig) string {
	if !cfg.RequiresCommand() {
		return ""
	}
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
	command := ""
	if cfg.RequiresCommand() {
		command = strings.TrimSpace(defaults.ACP[name].Command)
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
	}
	defaultModelProvider := strings.TrimSpace(cfg.ModelProvider)
	cfg.ModelProvider = strings.TrimSpace(defaults.ACP[name].ModelProvider)
	cfg.Model = strings.TrimSpace(defaults.ACP[name].Model)
	cfg.ReasoningEffort = strings.TrimSpace(defaults.ACP[name].ReasoningEffort)
	cfg.Auth = defaults.ACP[name].Auth
	if cfg.UsesModelProvider() {
		cfg = cfg.NormalizeProviderModel(defaultModelProvider)
	}
	return cfg, command, nil
}
