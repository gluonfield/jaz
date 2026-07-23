package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/httpapi"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type SettingsHandler struct {
	Store     storage.SettingsStorage
	Extension ExtensionStatusProvider
	OnChange  func()
}

type ExtensionStatusProvider interface {
	Status() browsercontrol.ExtensionStatus
}

type StatusResponse struct {
	Enabled   bool                           `json:"enabled"`
	Mode      string                         `json:"mode"`
	Extension browsercontrol.ExtensionStatus `json:"extension"`
}

type settingsInput struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Mode    *string `json:"mode,omitempty"`
}

func NewSettingsHandler(store storage.SettingsStorage, extension ExtensionStatusProvider, onChange func()) SettingsHandler {
	return SettingsHandler{Store: store, Extension: extension, OnChange: onChange}
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
	return StatusResponse{Enabled: settings.Enabled, Mode: jazsettings.BrowserMode(settings), Extension: extension}, nil
}

func (h SettingsHandler) extensionStatus() browsercontrol.ExtensionStatus {
	if h.Extension == nil {
		return browsercontrol.ExtensionStatus{}
	}
	return h.Extension.Status()
}

func (h SettingsHandler) normalize(input settingsInput) (jazsettings.BrowserSettings, error) {
	settings, err := jazsettings.LoadBrowserSettings(h.Store)
	if err != nil {
		return jazsettings.BrowserSettings{}, err
	}
	if input.Enabled != nil {
		settings.Enabled = *input.Enabled
	}
	if input.Mode != nil {
		mode := jazsettings.NormalizeBrowserMode(*input.Mode)
		if mode == "" {
			return jazsettings.BrowserSettings{}, fmt.Errorf("unknown browser mode %q", strings.TrimSpace(*input.Mode))
		}
		settings.Mode = mode
	}
	settings.Mode = jazsettings.BrowserMode(settings)
	return settings, nil
}
