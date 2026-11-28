//go:build browserintegration

package browsercontrol_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/browsercontrol"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	"github.com/wins/jaz/backend/internal/onboardingstate"
	serverapi "github.com/wins/jaz/backend/internal/server"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func mountBrowserStartupFixture(t *testing.T, mux *http.ServeMux, store *sqlitestore.Store, backend *browsercontrol.ConfiguredBackend) {
	t.Helper()
	if err := onboardingstate.Save(onboardingstate.Path(store.RootDir()), onboardingstate.State{Completed: true}); err != nil {
		t.Fatal(err)
	}
	const token = "jaz_browser-startup-fixture"
	thread, err := store.LoadSession("browser-background")
	if err != nil {
		t.Fatal(err)
	}
	handler := browserapi.DesktopHandler{Backend: backend.DesktopBackend, Store: store}
	server := httptest.NewServer((&serverapi.Server{
		Store: store, AuthKey: token,
		Routes: serverapi.Routes{
			{Pattern: "GET /v1/browser", Handler: browserapi.NewSettingsHandler(store, nil)},
			{Pattern: "GET /v1/browser/desktop", Handler: handler},
			{Pattern: "GET /v1/sessions/{session}/browser", Handler: handler},
			{Pattern: "POST /v1/sessions/{session}/browser", Handler: handler},
		},
	}).Handler())
	t.Cleanup(server.Close)
	mux.HandleFunc("GET /exercise-startup-backend", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"url": server.URL, "token": token, "session": thread.ID})
	})
}
