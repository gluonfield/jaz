package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/deviceauth"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	"github.com/wins/jaz/backend/internal/settings"
	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestPackagedBrowserOriginPreservesAuthentication(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := settings.SaveBrowserSettings(store, settings.BrowserSettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	devices := deviceauth.New(store)
	device, err := devices.RegisterApproved(testDeviceInfo("Desktop", "desktop", 1))
	if err != nil {
		t.Fatal(err)
	}
	backend := browsercontrol.NewDesktopBackend()
	defer backend.Close()
	srv := httptest.NewServer((&Server{
		Store: store, Devices: devices, AuthKey: "root",
		Routes: Routes{{Pattern: "GET /v1/browser/desktop", Handler: browserapi.DesktopHandler{Backend: backend, Store: store}}},
	}).Handler())
	defer srv.Close()
	for _, test := range []struct {
		name   string
		origin string
		token  string
		status int
	}{
		{"packaged desktop", "file://", device.Token, http.StatusSwitchingProtocols},
		{"missing credential", "file://", "", http.StatusUnauthorized},
		{"invalid credential", "file://", "invalid", http.StatusUnauthorized},
		{"untrusted website", "https://untrusted.example", device.Token, http.StatusForbidden},
		{"file host", "file://untrusted.example", device.Token, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			dialer := websocket.Dialer{Subprotocols: []string{"jaz", "jaz-auth." + test.token}}
			conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/browser/desktop", http.Header{"Origin": {test.origin}})
			if conn != nil {
				defer conn.Close()
			}
			if response == nil {
				t.Fatalf("no upgrade response: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.status || (test.status == http.StatusSwitchingProtocols && err != nil) {
				t.Fatalf("upgrade status=%d want=%d: %v", response.StatusCode, test.status, err)
			}
		})
	}
}
