package computer

import (
	"context"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/computercontrol"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type DesktopHandler struct {
	Backend *computercontrol.DesktopBackend
	Store   interface {
		storage.SettingsStorage
		LoadSession(string) (storage.Session, error)
	}
}

func (h DesktopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	config, err := settings.LoadComputerSettings(h.Store)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if !config.Enabled {
		http.Error(w, "enable Computer use in Jaz settings", http.StatusForbidden)
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
	connection, err := httpapi.UpgradeWebSocket(w, r)
	if err != nil {
		return
	}
	stop := context.AfterFunc(r.Context(), func() {
		_ = connection.Close()
	})
	defer stop()
	h.Backend.Connect(r.Context(), session.ID, connection)
}
