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
	"github.com/wins/jaz/backend/internal/managedtool"
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

type onboardingStateResponse struct {
	Completed bool `json:"completed"`
}

type onboardingACPProbe struct {
	acpAuthStatusResponse
	Agent                string                       `json:"agent"`
	Command              string                       `json:"command,omitempty"`
	Installed            bool                         `json:"installed"`
	AppInstalled         bool                         `json:"app_installed,omitempty"`
	AppName              string                       `json:"app_name,omitempty"`
	Available            bool                         `json:"available"`
	AuthCommand          string                       `json:"auth_command,omitempty"`
	AuthCommandAvailable bool                         `json:"auth_command_available"`
	AuthCommandReason    string                       `json:"auth_command_reason,omitempty"`
	ManagedAdapter       *onboardingACPAdapterStatus  `json:"managed_adapter,omitempty"`
	ManagedTool          *onboardingManagedToolStatus `json:"managed_tool,omitempty"`
}

type onboardingACPAdapterStatus struct {
	Adapter         string `json:"adapter"`
	Version         string `json:"version,omitempty"`
	Platform        string `json:"platform,omitempty"`
	State           string `json:"state"`
	Message         string `json:"message,omitempty"`
	BytesDownloaded int64  `json:"bytes_downloaded,omitempty"`
	BytesTotal      int64  `json:"bytes_total,omitempty"`
	ProgressPercent int    `json:"progress_percent,omitempty"`
}

type onboardingManagedToolStatus struct {
	Tool     string `json:"tool"`
	Version  string `json:"version,omitempty"`
	Platform string `json:"platform,omitempty"`
	State    string `json:"state"`
	Path     string `json:"path,omitempty"`
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
			next, err := agentsettings.NormalizeAgentDefaults(*input.Settings, s.acpAgentCatalog(), s.ModelCatalog)
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

func (s *Server) handleOnboardingState(w http.ResponseWriter, r *http.Request) {
	state, _, err := onboardingstate.Load(s.onboardingStatePath())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, onboardingStateResponse{Completed: state.Completed})
}

func (s *Server) handlePrepareACPAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	agent := acp.CanonicalAgentName(r.PathValue("agent"))
	cfg, ok := s.acpAgentCatalog().Agent(agent)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown acp agent %q", agent))
		return
	}
	if adapter := strings.TrimSpace(cfg.ManagedAdapter); adapter != "" {
		if s.ACPAdapters == nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("managed adapter downloader is not available"))
			return
		}
		if err := s.ACPAdapters.Prepare(r.Context(), adapter); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	if tool := strings.TrimSpace(cfg.ManagedTool); tool != "" {
		if s.ManagedTools == nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("managed tool downloader is not available"))
			return
		}
		if err := s.ManagedTools.Prepare(r.Context(), tool); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		settings.Agent = agentsettings.DefaultWorkerAgent(defaults)
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
		readiness := s.probeACPReadiness(name, cfg, auth)
		adapter := s.managedAdapterStatus(cfg)
		adapterReady := adapter == nil || adapter.State == acpadapter.StateReady
		tool := s.managedToolStatus(cfg.ManagedTool)
		toolReady := tool == nil || tool.State == managedtool.StateReady
		installed := (adapterInstalled || auth.LoginCommandAvailable) && adapterReady && toolReady
		reason := ""
		if !adapterReady && adapter != nil {
			reason = adapter.Message
		} else if !toolReady && tool != nil {
			reason = tool.Message
		} else if !installed {
			reason = firstMessage(commandMissingReason(cfg), auth.LoginCommandReason)
		} else if !adapterInstalled {
			reason = commandMissingReason(cfg)
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
			Available:             readiness.Available && adapterReady && toolReady,
			AuthCommand:           auth.LoginCommand,
			AuthCommandAvailable:  auth.LoginCommandAvailable,
			AuthCommandReason:     auth.LoginCommandReason,
			ManagedAdapter:        adapter,
			ManagedTool:           tool,
		})
	}
	return out
}

func (s *Server) probeACPReadiness(name string, cfg acp.AgentConfig, auth acp.AgentAuthStatus) acp.Readiness {
	if strings.TrimSpace(cfg.ManagedAdapter) == "" {
		return acp.ProbeReadinessWithProviders(name, cfg, s.runtimeRoot(), nil, s.modelProviders())
	}
	if !auth.Authenticated {
		return acp.Readiness{Reason: auth.Reason}
	}
	return acp.Readiness{Available: true}
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
		Adapter:         status.Adapter,
		Version:         status.Version,
		Platform:        status.Platform,
		State:           status.State,
		Message:         status.Message,
		BytesDownloaded: status.BytesDownloaded,
		BytesTotal:      status.BytesTotal,
		ProgressPercent: status.ProgressPercent,
	}
}

func (s *Server) managedToolStatus(tool string) *onboardingManagedToolStatus {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return nil
	}
	status := managedtool.Status{
		Tool:    tool,
		State:   managedtool.StateMissing,
		Message: "managed tool downloader is not available",
	}
	if s.ManagedTools != nil {
		status = s.ManagedTools.Status(tool)
	}
	return &onboardingManagedToolStatus{
		Tool:     status.Tool,
		Version:  status.Version,
		Platform: status.Platform,
		State:    status.State,
		Path:     status.Path,
		Message:  status.Message,
	}
}

// adapterBundleDir is the dir holding a managed adapter's binaries, or "".
func (s *Server) adapterBundleDir(adapter string) string {
	adapter = strings.TrimSpace(adapter)
	if adapter == "" || s.ACPAdapters == nil {
		return ""
	}
	path := strings.TrimSpace(s.ACPAdapters.Status(adapter).Path)
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

func (s *Server) managedToolBinDir(tool string) string {
	status := s.managedToolStatus(tool)
	if status == nil || status.State != managedtool.StateReady || strings.TrimSpace(status.Path) == "" {
		return ""
	}
	return filepath.Dir(status.Path)
}

func (s *Server) agentLoginBinDirs(cfg acp.AgentConfig) string {
	dirs := []string{}
	if dir := strings.TrimSpace(cfg.AdapterBinDir); dir != "" {
		dirs = append(dirs, dir)
	}
	if dir := strings.TrimSpace(s.managedToolBinDir(cfg.ManagedTool)); dir != "" {
		dirs = append(dirs, dir)
	}
	return strings.Join(dirs, string(os.PathListSeparator))
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
		// The launch command is catalog-owned, not user settings.
		command = agentsettings.CommandLine(cfg.Command, cfg.Args)
	}
	defaultModelProvider := strings.TrimSpace(cfg.ModelProvider)
	cfg.ModelProvider = strings.TrimSpace(defaults.ACP[name].ModelProvider)
	cfg.Model = strings.TrimSpace(defaults.ACP[name].Model)
	cfg.ReasoningEffort = strings.TrimSpace(defaults.ACP[name].ReasoningEffort)
	cfg.Auth = defaults.ACP[name].Auth
	if cfg.UsesModelProvider() {
		cfg = cfg.NormalizeProviderModel(defaultModelProvider)
	}
	cfg.AdapterBinDir = s.adapterBundleDir(cfg.ManagedAdapter)
	cfg.LoginBinDir = s.agentLoginBinDirs(cfg)
	return cfg, command, nil
}
