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
	"github.com/wins/jaz/backend/internal/connections"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	"github.com/wins/jaz/backend/internal/m365copilot"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"github.com/wins/jaz/backend/internal/visualize"
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
		fx.Supply(mcpruntime.NewManager(store, nil, tools.NewRegistry(), nil), Config{}, RuntimeAuthKey("secret"), serverconfig.Config{}, store, manager, acp.AgentCatalog{}, runtimefiles.New(store.RootDir())),
		fx.Provide(NewOAuthBroker, NewMicrosoftCopilotAuth, NewProviderSource, sessionevents.New, fx.Annotate(visualize.NewService, fx.From(new(*sqlitestore.Store), new(*sqlitestore.Store), new(*sessionevents.Bus)))),
		fx.Provide(func() storage.Store { return store }),
		fx.Provide(func() storage.PluginStore { return store }),
		fx.Provide(func() *connections.Catalog { return connections.NewCatalog(nil) }),
		fx.Provide(
			NewConnectionOAuthService,
			func() *connections.QRService { return connections.NewQRService() },
			NewConnectionMCPConnector,
			NewConnectionConnectService,
			func(catalog *connections.Catalog, store *sqlitestore.Store) *connections.Service {
				return connections.NewService(catalog, store)
			},
		),
		fx.Provide(func() storage.UsageEventStore { return fakeUsageStore{} }),
		fx.Provide(func() storage.FeedStore { return fakeFeedStore{} }),
		HTTPModule(),
		fx.Populate(&routes, &publicRoutes),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	requireRoute(t, routes, "GET /v1/usage/daily")
	requireRoute(t, routes, "GET "+m365copilot.OAuthCallbackPath)
	requireRoute(t, routes, "POST /v1/filesystem/dirs")
	requireRoute(t, routes, "GET /v1/usage/models")
	requireRoute(t, routes, "GET /v1/feed")
	requireRoute(t, routes, "GET /v1/sessions")
	requireRoute(t, routes, "GET /v1/notifications/stream")
	requireRoute(t, routes, "GET /v1/sessions/{session}/messages")
	requireRoute(t, routes, "GET /v1/sessions/{session}/overview")
	requireRoute(t, routes, "POST /v1/sessions/{session}/plugin-views")
	requireRoute(t, routes, "DELETE /v1/sessions/{session}/goal")
	requireRoute(t, routes, "POST /v1/sessions/{session}/agent/input")
	requireRoute(t, routes, "PUT /v1/sessions/{session}/mcp/discovery")
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

func TestNewRoutesIncludesUnifiedPluginRoutes(t *testing.T) {
	catalog := connections.NewCatalog(nil)
	oauth := connections.NewOAuthService(fakeConnectionOAuthStore{}, connections.OAuthConfig{})
	qr := connections.NewQRService()
	connService := connections.NewService(catalog, fakeConnectionOAuthStore{})
	connect := connections.NewConnectService(catalog, oauth, qr, nil)
	pluginService := plugins.NewService(plugins.New(runtimefiles.New(t.TempDir())), nil, connService, connect, oauth, qr)
	routes := NewRoutes(routeDeps{
		Usage:   usagecore.NewService(fakeUsageStore{}),
		Plugins: pluginService,
		Preview: testPreviewHandler(t),
	})
	want := []string{
		"GET /v1/plugins",
		"GET /v1/plugins/{plugin}",
		"PUT /v1/plugins/{plugin}/enabled",
		"PUT /v1/plugins/{plugin}/config",
		"POST /v1/plugins/{plugin}/connect",
		"DELETE /v1/plugins/accounts/{id}",
		"GET /v1/plugins/qr/{id}/status",
		"POST /v1/plugins/qr/{id}/password",
		"DELETE /v1/plugins/qr/{id}",
		"GET /v1/plugins/oauth/callback",
		"GET /v1/plugins/sources",
		"POST /v1/plugins/sources",
		"DELETE /v1/plugins/sources/{id}",
		"POST /v1/plugins/sources/refresh",
		"POST /v1/plugins/install",
		"POST /v1/plugins/{plugin}/update",
		"DELETE /v1/plugins/{plugin}",
		"GET /v1/plugins/{plugin}/page",
	}
	found := map[string]bool{}
	for _, route := range routes {
		if route.Handler != nil {
			found[route.Pattern] = true
		}
	}
	for _, pattern := range want {
		if !found[pattern] {
			t.Fatalf("missing plugin route %q: %#v", pattern, routes)
		}
	}
}

func TestHTTPModuleWiresWithNewStore(t *testing.T) {
	var routes server.Routes
	var publicRoutes server.PublicRoutes
	var store *sqlitestore.Store
	app := fx.New(
		fx.NopLogger,
		fx.Supply(runtimefiles.New(t.TempDir()), acp.AgentCatalog{}, Config{}, serverconfig.Config{}),
		fx.Provide(NewStore, NewOAuthBroker, NewMicrosoftCopilotAuth, NewRuntimeAuthKey, NewProviderSource, sessionevents.New, fx.Annotate(visualize.NewService, fx.From(new(*sqlitestore.Store), new(*sqlitestore.Store), new(*sessionevents.Bus)))),
		fx.Provide(
			func() *connections.Catalog { return connections.NewCatalog(nil) },
			NewConnectionOAuthService,
			func() *connections.QRService { return connections.NewQRService() },
			NewConnectionMCPConnector,
			NewConnectionConnectService,
			func(catalog *connections.Catalog, store *sqlitestore.Store) *connections.Service {
				return connections.NewService(catalog, store)
			},
		),
		fx.Provide(func(store *sqlitestore.Store) *mcpruntime.Manager {
			return mcpruntime.NewManager(store, nil, tools.NewRegistry(), nil)
		}),
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
