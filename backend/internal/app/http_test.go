package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/deviceauth"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"go.uber.org/fx"
)

type fakeUsageStore struct{}

func (fakeUsageStore) UsageEvents(time.Time, time.Time) ([]storage.UsageEvent, error) {
	return nil, nil
}

type fakeFeedStore struct{}

func (fakeFeedStore) LoadFeed() ([]storage.FeedItem, error) { return nil, nil }

func (fakeFeedStore) LoadFeedCompletions() ([]storage.FeedCompletion, error) { return nil, nil }

type fakeConnectionOAuthStore struct{}

func (fakeConnectionOAuthStore) LoadToken(context.Context, string) (integrationoauth.Token, bool, error) {
	return integrationoauth.Token{}, false, nil
}

func (fakeConnectionOAuthStore) LoadConnection(context.Context, string) (integrations.Connection, bool, error) {
	return integrations.Connection{}, false, nil
}

func (fakeConnectionOAuthStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return nil, nil
}

func (fakeConnectionOAuthStore) SaveOAuthConnection(context.Context, integrationoauth.Token, integrations.Connection) error {
	return nil
}

func (fakeConnectionOAuthStore) DeleteConnection(context.Context, string) (bool, error) {
	return false, nil
}

func TestHTTPModuleProvidesRoute(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := acp.NewManager(store, acp.Config{}, nil)
	var routes server.Routes
	var publicRoutes server.PublicRoutes
	app := fx.New(
		fx.NopLogger,
		fx.Supply(Config{}, RuntimeAuthKey("secret"), serverconfig.Config{}, store, manager, acp.AgentCatalog{}, runtimefiles.New(store.RootDir())),
		fx.Provide(NewProviderSource, sessionevents.New),
		fx.Provide(func() storage.Store { return store }),
		fx.Provide(func() storage.UsageEventStore { return fakeUsageStore{} }),
		fx.Provide(func() storage.FeedStore { return fakeFeedStore{} }),
		HTTPModule(),
		fx.Populate(&routes, &publicRoutes),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	requireRoute(t, routes, "GET /v1/usage/daily")
	requireRoute(t, routes, "POST /v1/filesystem/dirs")
	requireRoute(t, routes, "GET /v1/usage/models")
	requireRoute(t, routes, "GET /v1/feed")
	requireRoute(t, routes, "GET /v1/sessions")
	requireRoute(t, routes, "GET /v1/feed/completions")
	requireRoute(t, routes, "GET /v1/sessions/{session}/messages")
	requireRoute(t, routes, "GET /v1/sessions/{session}/overview")
	requireRoute(t, routes, "DELETE /v1/sessions/{session}/goal")
	requireRoute(t, routes, "POST /v1/sessions/{session}/agent/input")
	requireRoute(t, routes, "GET /v1/settings/voice")
	requireRoute(t, routes, "PUT /v1/settings/voice")
	requireRoute(t, routes, "POST /v1/voice/connect")
	requireRoute(t, routes, "POST /v1/sessions/{session}/voice/transcript")
	requireRoute(t, routes, "/v1/preview/")
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions?archived=true&limit=1", nil)
	request.Header.Set("Authorization", "Bearer secret")
	(&server.Server{Store: store, Routes: routes, AuthKey: "secret"}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session list status = %d: %s", response.Code, response.Body.String())
	}
	parent := t.TempDir()
	body, err := json.Marshal(map[string]string{"parent": parent, "name": "new project"})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&server.Server{Store: store, Routes: routes, AuthKey: "secret"}).Handler()
	for _, authenticated := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "/v1/filesystem/dirs", strings.NewReader(string(body)))
		want := http.StatusUnauthorized
		if authenticated {
			request.Header.Set("Authorization", "Bearer secret")
			want = http.StatusOK
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("create directory status = %d: %s", response.Code, response.Body.String())
		}
		if !authenticated {
			if _, err := os.Stat(filepath.Join(parent, "new project")); !os.IsNotExist(err) {
				t.Fatalf("unauthenticated request created a directory: %v", err)
			}
			continue
		}
		request = httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(response.Body.String()))
		request.Header.Set("Authorization", "Bearer secret")
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("save project status = %d: %s", response.Code, response.Body.String())
		}
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var saved struct {
		Projects []struct {
			Path string `json:"path"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Projects) != 1 || saved.Projects[0].Path != filepath.Join(parent, "new project") {
		t.Fatalf("saved projects = %#v", saved.Projects)
	}
	if len(publicRoutes) != 1 ||
		publicRoutes[0].Match == nil ||
		publicRoutes[0].Handler == nil {
		t.Fatalf("publicRoutes = %#v", publicRoutes)
	}
}

func TestNewRoutesMountsDeviceRevokeAsMethodRoute(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	routes := NewRoutes(routeDeps{
		Usage:   usagecore.NewService(fakeUsageStore{}),
		Devices: deviceauth.New(store),
		Store:   store,
		Preview: testPreviewHandler(t),
	})
	var foundRevoke bool
	connectionMethods := map[string]bool{}
	for _, route := range routes {
		if route.Pattern == "/v1/devices/" {
			t.Fatalf("mounted generic device item route")
		}
		if route.Pattern == "DELETE /v1/devices/{id}" && route.Handler != nil {
			foundRevoke = true
		}
		if (route.Pattern == "GET /v1/devices/connection-link" || route.Pattern == "PUT /v1/devices/connection-link") && route.Handler != nil {
			connectionMethods[route.Pattern] = true
		}
	}
	if !foundRevoke {
		t.Fatalf("missing DELETE device revoke route: %#v", routes)
	}
	if len(connectionMethods) != 2 {
		t.Fatalf("device connection link methods = %#v", connectionMethods)
	}
}

func TestNewRoutesDisablePairingGatesPairingRoutes(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mounted := func(disable bool) (pairing, register bool) {
		routes := NewRoutes(routeDeps{
			Usage:   usagecore.NewService(fakeUsageStore{}),
			Jaz:     Config{Devices: DevicesConfig{DisablePairing: disable}},
			Devices: deviceauth.New(store),
			Store:   store,
			Preview: testPreviewHandler(t),
		})
		for _, route := range routes {
			switch route.Pattern {
			case "POST /v1/devices/pairing-requests", "/v1/devices/pairing-requests/":
				pairing = true
			case "POST /v1/devices/register":
				register = register || route.Handler != nil
			}
		}
		return
	}

	if pairing, register := mounted(false); !pairing || !register {
		t.Fatalf("default: pairing=%v register=%v, want both mounted", pairing, register)
	}
	if pairing, register := mounted(true); pairing || !register {
		t.Fatalf("disabled: pairing=%v register=%v, want only register", pairing, register)
	}
}

func TestNewRoutesIncludesBrowserExtensionRoute(t *testing.T) {
	routes := NewRoutes(routeDeps{
		Usage:   usagecore.NewService(fakeUsageStore{}),
		Browser: &browsercontrol.ConfiguredBackend{ExtensionBridge: browsercontrol.NewExtensionBridge()},
		Preview: testPreviewHandler(t),
	})
	for _, route := range routes {
		if route.Pattern == "GET /v1/browser/extension" && route.Handler != nil {
			return
		}
	}
	t.Fatalf("missing browser extension route: %#v", routes)
}

func TestNewRoutesIncludesBrowserSettingsRoutes(t *testing.T) {
	routes := NewRoutes(routeDeps{
		Usage:           usagecore.NewService(fakeUsageStore{}),
		BrowserSettings: &BrowserSettingsHandler{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})},
		Preview:         testPreviewHandler(t),
	})
	found := map[string]bool{}
	for _, route := range routes {
		if (route.Pattern == "GET /v1/browser" || route.Pattern == "PUT /v1/browser") && route.Handler != nil {
			found[route.Pattern] = true
		}
	}
	if !found["GET /v1/browser"] || !found["PUT /v1/browser"] {
		t.Fatalf("missing browser settings routes: %#v", routes)
	}
}

func TestNewRoutesIncludesConnectionPluginRoutes(t *testing.T) {
	catalog := connections.NewCatalog()
	oauth := connections.NewOAuthService(fakeConnectionOAuthStore{}, connections.OAuthConfig{})
	qr := connections.NewQRService()
	routes := NewRoutes(routeDeps{
		Usage:           usagecore.NewService(fakeUsageStore{}),
		Connections:     connections.NewService(catalog, fakeConnectionOAuthStore{}, nil),
		ConnectionOAuth: oauth,
		ConnectionQR:    qr,
		ConnectionStart: connections.NewConnectService(catalog, oauth, qr, nil, nil),
		Preview:         testPreviewHandler(t),
	})
	found := map[string]bool{}
	for _, route := range routes {
		if (route.Pattern == "GET /v1/connections/plugins" ||
			route.Pattern == "GET /v1/connections/plugins/{id}" ||
			route.Pattern == "DELETE /v1/connections/accounts/{id}" ||
			route.Pattern == "POST /v1/connections/plugins/{id}/connect" ||
			route.Pattern == "GET /v1/connections/qr/{id}" ||
			route.Pattern == "POST /v1/connections/qr/{id}/password" ||
			route.Pattern == "DELETE /v1/connections/qr/{id}" ||
			route.Pattern == "GET /v1/connections/oauth/callback") && route.Handler != nil {
			found[route.Pattern] = true
		}
	}
	if !found["GET /v1/connections/plugins"] ||
		!found["GET /v1/connections/plugins/{id}"] ||
		!found["DELETE /v1/connections/accounts/{id}"] ||
		!found["POST /v1/connections/plugins/{id}/connect"] ||
		!found["GET /v1/connections/qr/{id}"] ||
		!found["POST /v1/connections/qr/{id}/password"] ||
		!found["DELETE /v1/connections/qr/{id}"] ||
		!found["GET /v1/connections/oauth/callback"] {
		t.Fatalf("missing connection plugin routes: %#v", routes)
	}
}

func TestNewRoutesKeepsConnectionStartRoutesIndependent(t *testing.T) {
	catalog := connections.NewCatalog()
	oauth := connections.NewOAuthService(fakeConnectionOAuthStore{}, connections.OAuthConfig{})
	routes := NewRoutes(routeDeps{
		Usage:           usagecore.NewService(fakeUsageStore{}),
		Connections:     connections.NewService(catalog, fakeConnectionOAuthStore{}, nil),
		ConnectionOAuth: oauth,
		Preview:         testPreviewHandler(t),
	})
	for _, route := range routes {
		if route.Pattern == "GET /v1/connections/oauth/callback" && route.Handler != nil {
			return
		}
	}
	t.Fatalf("missing OAuth callback route without QR service: %#v", routes)
}

func TestHTTPModuleWiresWithNewStore(t *testing.T) {
	var routes server.Routes
	var publicRoutes server.PublicRoutes
	var store *sqlitestore.Store
	app := fx.New(
		fx.NopLogger,
		fx.Supply(runtimefiles.New(t.TempDir()), acp.AgentCatalog{}, Config{}, serverconfig.Config{}),
		fx.Provide(NewStore, NewRuntimeAuthKey, NewProviderSource, sessionevents.New),
		fx.Provide(func(store *sqlitestore.Store) *acp.Manager {
			return acp.NewManager(store, acp.Config{}, nil)
		}),
		HTTPModule(),
		fx.Populate(&routes, &publicRoutes, &store),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	requireRoute(t, routes, "GET /v1/feed")
	requireRoute(t, routes, "/v1/preview/")
	if len(publicRoutes) != 1 {
		t.Fatalf("publicRoutes = %#v", publicRoutes)
	}
	for _, route := range routes {
		if route.Handler == nil {
			t.Fatalf("nil handler in routes = %#v", routes)
		}
	}
	router := &server.Server{Store: store, Routes: routes, PublicRoutes: publicRoutes, AuthKey: "fixture-key"}
	listener := httptest.NewServer(router.Handler())
	defer listener.Close()
	request, err := http.NewRequest(http.MethodGet, listener.URL+"/v1/mobile/push", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer fixture-key")
	response, err := listener.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var push struct {
		Configured bool `json:"configured"`
	}
	if err := json.NewDecoder(response.Body).Decode(&push); err != nil || response.StatusCode != http.StatusOK || push.Configured {
		t.Fatalf("push route did not start with the real router: status=%d configured=%v error=%v", response.StatusCode, push.Configured, err)
	}
}

func testPreviewHandler(t *testing.T) *previewapi.Handler {
	t.Helper()
	handler, err := previewapi.NewHandler(serverconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func requireRoute(t *testing.T, routes server.Routes, pattern string) {
	t.Helper()
	for _, route := range routes {
		if route.Pattern == pattern && route.Handler != nil {
			return
		}
	}
	t.Fatalf("missing route %q in %#v", pattern, routes)
}
