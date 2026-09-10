package browser

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type DesktopHandler struct {
	Backend *browsercontrol.DesktopBackend
	Store   interface {
		storage.SettingsStorage
		LoadSession(string) (storage.Session, error)
	}
}

func (h DesktopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	config, err := settings.LoadBrowserSettings(h.Store)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if !config.Enabled || config.Mode != settings.BrowserModeDesktop {
		http.Error(w, "enable the Jaz side browser in Browser settings", http.StatusForbidden)
		return
	}
	session, err := h.Store.LoadSession(r.PathValue("session"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		httpapi.WriteError(w, status, err)
		return
	}
	if r.Method == http.MethodPost {
		var input browsercontrol.ActionInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 150000)).Decode(&input); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		if input.Action == browsercontrol.ActionScript {
			http.Error(w, "nested browser scripts are unsupported", http.StatusBadRequest)
			return
		}
		input, err = browsercontrol.NormalizeActionInput(input)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		input.Session = session.ID
		out, err := h.Backend.Call(r.Context(), input)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, out)
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.Backend.Connect(r.Context(), session.ID, conn)
}
