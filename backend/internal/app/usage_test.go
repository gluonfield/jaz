package app

import (
	"context"
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
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"go.uber.org/fx"
)

type fakeUsageStore struct{}

func (fakeUsageStore) UsageEventsSince(time.Time) ([]storage.UsageEvent, error) {
	return nil, nil
}

type fakeFeedStore struct{}

func (fakeFeedStore) LoadFeed() ([]storage.FeedItem, error) { return nil, nil }

func (fakeFeedStore) SetThreadUnread(string, bool) error { return nil }

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

func TestUsageModuleProvidesRoute(t *testing.T) {
	var routes server.Routes
	app := fx.New(
		fx.NopLogger,
		fx.Supply(Config{}),
		fx.Provide(func() storage.UsageEventStore { return fakeUsageStore{} }),
		fx.Provide(func() storage.FeedStore { return fakeFeedStore{} }),
		UsageModule(),
		fx.Populate(&routes),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 3 ||
		routes[0].Pattern != "GET /v1/usage/daily" ||
		routes[0].Handler == nil ||
		routes[1].Pattern != "GET /v1/usage/models" ||
		routes[1].Handler == nil ||
		routes[2].Pattern != "GET /v1/feed" ||
		routes[2].Handler == nil {
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
	catalog := connections.NewCatalog()
	oauth := connections.NewOAuthService(fakeConnectionOAuthStore{}, connections.OAuthConfig{})
	qr := connections.NewQRService()
	routes := NewRoutes(routeDeps{
		Usage:           usagecore.NewService(fakeUsageStore{}),
		Connections:     connections.NewService(catalog, fakeConnectionOAuthStore{}),
		ConnectionOAuth: oauth,
		ConnectionQR:    qr,
		ConnectionStart: connections.NewConnectService(catalog, oauth, qr),
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
		Connections:     connections.NewService(catalog, fakeConnectionOAuthStore{}),
		ConnectionOAuth: oauth,
	})
	for _, route := range routes {
		if route.Pattern == "GET /v1/connections/oauth/callback" && route.Handler != nil {
			return
		}
	}
	t.Fatalf("missing OAuth callback route without QR service: %#v", routes)
}

func TestUsageModuleWiresWithNewStore(t *testing.T) {
	var routes server.Routes
	var store *sqlitestore.Store
	app := fx.New(
		fx.NopLogger,
		fx.Supply(runtimefiles.New(t.TempDir()), acp.AgentCatalog{}, Config{}),
		fx.Provide(NewStore),
		UsageModule(),
		fx.Populate(&routes, &store),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if len(routes) != 3 {
		t.Fatalf("routes = %#v", routes)
	}
	var hasFeed bool
	for _, route := range routes {
		if route.Handler == nil {
			t.Fatalf("nil handler in routes = %#v", routes)
		}
		if route.Pattern == "GET /v1/feed" {
			hasFeed = true
		}
	}
	if !hasFeed {
		t.Fatalf("feed route missing from routes = %#v", routes)
	}
}
