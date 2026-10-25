package browser

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type SettingsHandler struct {
	Store    storage.SettingsStorage
	OnChange func()
}

func NewSettingsHandler(store storage.SettingsStorage, onChange func()) SettingsHandler {
	return SettingsHandler{Store: store, OnChange: onChange}
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
	current, err := settings.LoadBrowserSettings(h.Store)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, current)
}

func (h SettingsHandler) update(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if input.Enabled == nil {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("enabled is required"))
		return
	}
	current, err := settings.SaveBrowserSettings(h.Store, settings.BrowserSettings{Enabled: *input.Enabled})
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if h.OnChange != nil {
		h.OnChange()
	}
	httpapi.WriteJSON(w, http.StatusOK, current)
}
