package app

import (
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/deviceauth"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	connectionsapi "github.com/wins/jaz/backend/internal/httpapi/connections"
	deviceapi "github.com/wins/jaz/backend/internal/httpapi/devices"
	feedapi "github.com/wins/jaz/backend/internal/httpapi/feed"
	modelcapabilitiesapi "github.com/wins/jaz/backend/internal/httpapi/modelcapabilities"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

type routeDeps struct {
	fx.In

	Usage           usagecore.Service
	Feed            feedcore.Service
	ModelCatalog    *modelcatalog.Service `optional:"true"`
	Jaz             Config
	Devices         *deviceauth.Service `optional:"true"`
	AuthKey         RuntimeAuthKey
	Config          serverconfig.Config            `optional:"true"`
	Browser         *browserworker.ExtensionBridge `optional:"true"`
	BrowserSettings *BrowserSettingsHandler        `optional:"true"`
	Connections     *connections.Service           `optional:"true"`
	ConnectionStart *connections.ConnectService    `optional:"true"`
	ConnectionOAuth *connections.OAuthService      `optional:"true"`
	ConnectionQR    *connections.QRService         `optional:"true"`
	MCP             *mcpruntime.Manager            `optional:"true"`
	Preview         *previewapi.Handler
	SessionMessages *sessionsapi.MessagesHandler
	SessionOverview *sessionsapi.OverviewHandler
}

func NewRoutes(deps routeDeps) server.Routes {
	routes := server.Routes{
		{Pattern: "GET /v1/sessions/{session}/messages", Handler: deps.SessionMessages},
		{Pattern: "GET /v1/sessions/{session}/overview", Handler: deps.SessionOverview},
	}
	routes = append(routes, usageRoutes(deps.Usage)...)
	routes = append(routes, feedRoutes(deps.Feed)...)
	routes = append(routes, modelCapabilityRoutes(deps.ModelCatalog)...)
	routes = appendConnectionRoutes(routes, deps.Connections, deps.ConnectionStart, deps.ConnectionOAuth, deps.ConnectionQR, deps.MCP, deps.Config)
	routes = appendDeviceRoutes(routes, deps.Devices, deps.Config, string(deps.AuthKey), deps.Jaz.Devices.DisablePairing)
	routes = appendBrowserRoutes(routes, deps.BrowserSettings, deps.Browser)
	return append(routes, server.Route{Pattern: "/v1/preview/", Handler: deps.Preview})
}

func modelCapabilityRoutes(catalog *modelcatalog.Service) server.Routes {
	if catalog == nil {
		return nil
	}
	handler := modelcapabilitiesapi.NewHandler(catalog)
	return server.Routes{
		{
			Pattern: "GET /v1/model-providers/{provider}/models",
			Handler: httpHandlerFunc(handler.ProviderModels),
		},
	}
}

func feedRoutes(feed feedcore.Service) server.Routes {
	handler := feedapi.NewHandler(feed)
	return server.Routes{
		{
			Pattern: "GET /v1/feed",
			Handler: httpHandlerFunc(handler.List),
		},
		{
			Pattern: "GET /v1/feed/completions",
			Handler: httpHandlerFunc(handler.Completions),
		},
	}
}

func usageRoutes(usage usagecore.Service) server.Routes {
	return server.Routes{
		{
			Pattern: "GET /v1/usage/daily",
			Handler: usageapi.NewDailyHandler(usage),
		},
		{
			Pattern: "GET /v1/usage/models",
			Handler: usageapi.NewModelsHandler(usage),
		},
	}
}

func oauthCallbackBaseURL(cfg serverconfig.Config) string {
	if strings.TrimSpace(cfg.PublicURL) == "" {
		return ""
	}
	return serverconfig.ClientBaseURL(cfg)
}

func appendConnectionRoutes(routes server.Routes, service *connections.Service, connect *connections.ConnectService, oauth *connections.OAuthService, qr *connections.QRService, mcp connectionsapi.MCPRefresher, cfg serverconfig.Config) server.Routes {
	if service == nil {
		return routes
	}
	handler := connectionsapi.NewPluginHandler(service, mcp)
	routes = append(routes,
		server.Route{Pattern: "GET /v1/connections/plugins", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "GET /v1/connections/plugins/{id}", Handler: httpHandlerFunc(handler.Get)},
		server.Route{Pattern: "DELETE /v1/connections/accounts/{id}", Handler: httpHandlerFunc(handler.Disconnect)},
	)
	if connect != nil || oauth != nil || qr != nil {
		connectHandler := connectionsapi.NewConnectHandler(connect, oauth, qr, mcp, oauthCallbackBaseURL(cfg))
		if connect != nil {
			routes = append(routes, server.Route{Pattern: "POST /v1/connections/plugins/{id}/connect", Handler: httpHandlerFunc(connectHandler.Start)})
		}
		if qr != nil {
			routes = append(routes, server.Route{Pattern: "GET /v1/connections/qr/{id}", Handler: httpHandlerFunc(connectHandler.QRStatus)})
			routes = append(routes, server.Route{Pattern: "POST /v1/connections/qr/{id}/password", Handler: httpHandlerFunc(connectHandler.QRPassword)})
			routes = append(routes, server.Route{Pattern: "DELETE /v1/connections/qr/{id}", Handler: httpHandlerFunc(connectHandler.CloseQR)})
		}
		if oauth != nil {
			routes = append(routes, server.Route{Pattern: "GET " + connectionsapi.OAuthCallbackPath, Handler: httpHandlerFunc(connectHandler.Callback)})
		}
	}
	return routes
}

func appendDeviceRoutes(routes server.Routes, devices *deviceauth.Service, cfg serverconfig.Config, authKey string, disablePairing bool) server.Routes {
	if devices == nil {
		return routes
	}
	handler := deviceapi.NewHandler(devices, cfg, authKey)
	routes = append(routes,
		server.Route{Pattern: "GET /v1/devices/connection-link", Handler: httpHandlerFunc(handler.ConnectionLink)},
		server.Route{Pattern: "GET /v1/devices", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "POST /v1/devices/register", Handler: httpHandlerFunc(handler.Register)},
		server.Route{Pattern: "DELETE /v1/devices/{id}", Handler: httpHandlerFunc(handler.Revoke)},
	)
	// Pairing is the unauthenticated keyless-onboarding surface.
	if disablePairing {
		return routes
	}
	return append(routes,
		server.Route{Pattern: "POST /v1/devices/pairing-requests", Handler: httpHandlerFunc(handler.CreatePairing)},
		server.Route{Pattern: "/v1/devices/pairing-requests/", Handler: httpHandlerFunc(handler.Pairing)},
	)
}

func appendBrowserRoutes(routes server.Routes, settings *BrowserSettingsHandler, extension *browserworker.ExtensionBridge) server.Routes {
	if settings != nil {
		routes = append(routes,
			server.Route{Pattern: "GET /v1/browser", Handler: settings},
			server.Route{Pattern: "PUT /v1/browser", Handler: settings},
		)
	}
	if extension != nil {
		routes = append(routes, server.Route{Pattern: "GET /v1/browser/extension", Handler: extension})
	}
	return routes
}

func NewPublicRoutes(handler *previewapi.Handler) server.PublicRoutes {
	return server.PublicRoutes{
		{Match: handler.IsPublicHostRequest, Handler: handler},
	}
}

type httpHandlerFunc func(http.ResponseWriter, *http.Request)

func (f httpHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}
