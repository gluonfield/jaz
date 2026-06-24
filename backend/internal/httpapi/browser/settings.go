package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/httpapi"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type SettingsHandler struct {
	Store     storage.SettingsStorage
	Catalog   acp.AgentCatalog
	Extension ExtensionStatusProvider
	OnChange  func()
}

type ExtensionStatusProvider interface {
	Status() browserworker.ExtensionStatus
}

type StatusResponse struct {
	Enabled   bool                          `json:"enabled"`
	Agent     string                        `json:"agent,omitempty"`
	Mode      string                        `json:"mode"`
	Extension browserworker.ExtensionStatus `json:"extension"`
}

type settingsInput struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Agent   *string `json:"agent,omitempty"`
	Mode    *string `json:"mode,omitempty"`
}

func NewSettingsHandler(store storage.SettingsStorage, catalog acp.AgentCatalog, extension ExtensionStatusProvider, onChange func()) SettingsHandler {
	return SettingsHandler{Store: store, Catalog: catalog, Extension: extension, OnChange: onChange}
}

func (h SettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.status(w)
	case http.MethodPut:
		h.update(w, r)
	default:
		httpapi.WriteError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (h SettingsHandler) status(w http.ResponseWriter) {
	status, err := h.browserStatus()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h SettingsHandler) update(w http.ResponseWriter, r *http.Request) {
	var input settingsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := h.normalize(input)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := jazsettings.SaveBrowserSettings(h.Store, settings); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if h.OnChange != nil {
		h.OnChange()
	}
	status, err := h.browserStatus()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h SettingsHandler) browserStatus() (StatusResponse, error) {
	settings, err := jazsettings.LoadBrowserSettings(h.Store)
	if err != nil {
		return StatusResponse{}, err
	}
	extension := h.extensionStatus()
	return StatusResponse{Enabled: settings.Enabled, Agent: settings.Agent, Mode: jazsettings.BrowserMode(settings), Extension: extension}, nil
}

func (h SettingsHandler) extensionStatus() browserworker.ExtensionStatus {
	if h.Extension == nil {
		return browserworker.ExtensionStatus{}
	}
	return h.Extension.Status()
}

func (h SettingsHandler) normalize(input settingsInput) (jazsettings.BrowserSettings, error) {
	settings, err := jazsettings.LoadBrowserSettings(h.Store)
	if err != nil {
		return jazsettings.BrowserSettings{}, err
	}
	extension := h.extensionStatus()
	if input.Enabled != nil {
		settings.Enabled = *input.Enabled
	}
	if input.Agent != nil {
		settings.Agent = acp.CanonicalAgentName(*input.Agent)
	}
	if input.Mode != nil {
		mode := jazsettings.NormalizeBrowserMode(*input.Mode)
		if mode == "" {
			return jazsettings.BrowserSettings{}, fmt.Errorf("unknown browser mode %q", strings.TrimSpace(*input.Mode))
		}
		settings.Mode = mode
	}
	settings.Mode = jazsettings.BrowserMode(settings)
	if settings.Enabled && jazsettings.BrowserUsesExtension(settings) && !extension.Connected && ((input.Enabled != nil && *input.Enabled) || input.Mode != nil) {
		return jazsettings.BrowserSettings{}, fmt.Errorf("connect the Chrome extension before enabling extension browser mode")
	}
	if strings.TrimSpace(settings.Agent) == "" {
		return settings, nil
	}
	agentSettings, err := jazsettings.LoadEffectiveAgentDefaults(h.Store, h.catalog())
	if err != nil {
		return jazsettings.BrowserSettings{}, err
	}
	if settings.Agent == acp.AgentJaz {
		return jazsettings.BrowserSettings{}, fmt.Errorf("built-in Jaz cannot be used as the browser agent yet")
	}
	if err := validateAgent(agentSettings, settings.Agent); err != nil {
		return jazsettings.BrowserSettings{}, err
	}
	return settings, nil
}

func (h SettingsHandler) catalog() acp.AgentCatalog {
	if len(h.Catalog) > 0 {
		return h.Catalog
	}
	return acp.BuiltinAgents()
}

func validateAgent(agentSettings jazsettings.AgentDefaults, agent string) error {
	if agent == "" {
		return nil
	}
	current, ok := agentSettings.ACP[agent]
	if !ok {
		return fmt.Errorf("unknown browser agent %q", agent)
	}
	if !current.Enabled {
		return fmt.Errorf("browser agent %q is not enabled", agent)
	}
	return nil
}
