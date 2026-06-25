package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

type fakeUsageStore struct{}

func (fakeUsageStore) UsageEventsSince(time.Time) ([]storage.UsageEvent, error) {
	return nil, nil
}

func TestUsageModuleProvidesRoute(t *testing.T) {
	var routes server.Routes
	app := fx.New(
		fx.NopLogger,
		fx.Provide(func() storage.UsageEventStore { return fakeUsageStore{} }),
		UsageModule(),
		fx.Populate(&routes),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 ||
		routes[0].Pattern != "GET /v1/usage/daily" ||
		routes[0].Handler == nil ||
		routes[1].Pattern != "GET /v1/usage/models" ||
		routes[1].Handler == nil {
		t.Fatalf("routes = %#v", routes)
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
	})
	var foundRevoke bool
	var foundConnection bool
	for _, route := range routes {
		if route.Pattern == "/v1/devices/" {
			t.Fatalf("mounted generic device item route")
		}
		if route.Pattern == "DELETE /v1/devices/{id}" && route.Handler != nil {
			foundRevoke = true
		}
		if route.Pattern == "GET /v1/devices/connection-link" && route.Handler != nil {
			foundConnection = true
		}
	}
	if !foundRevoke {
		t.Fatalf("missing DELETE device revoke route: %#v", routes)
	}
	if !foundConnection {
		t.Fatalf("missing device connection link route: %#v", routes)
	}
}

func TestNewRoutesIncludesBrowserExtensionRoute(t *testing.T) {
	routes := NewRoutes(routeDeps{
		Usage:   usagecore.NewService(fakeUsageStore{}),
		Browser: browserworker.NewExtensionBridge(nil, nil),
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
	routes := NewRoutes(routeDeps{
		Usage:       usagecore.NewService(fakeUsageStore{}),
		Connections: connections.NewCatalog(),
	})
	found := map[string]bool{}
	for _, route := range routes {
		if (route.Pattern == "GET /v1/connections/plugins" ||
			route.Pattern == "GET /v1/connections/plugins/{id}") && route.Handler != nil {
			found[route.Pattern] = true
		}
	}
	if !found["GET /v1/connections/plugins"] || !found["GET /v1/connections/plugins/{id}"] {
		t.Fatalf("missing connection plugin routes: %#v", routes)
	}
}

func TestUsageModuleWiresWithNewStore(t *testing.T) {
	var routes server.Routes
	var store *sqlitestore.Store
	app := fx.New(
		fx.NopLogger,
		fx.Supply(runtimefiles.New(t.TempDir()), acp.AgentCatalog{}),
		fx.Provide(NewStore),
		UsageModule(),
		fx.Populate(&routes, &store),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if len(routes) != 2 || routes[0].Handler == nil || routes[1].Handler == nil {
		t.Fatalf("routes = %#v", routes)
	}
}
