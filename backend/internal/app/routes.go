package app

import (
	"net/http"

	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/deviceauth"
	connectionsapi "github.com/wins/jaz/backend/internal/httpapi/connections"
	deviceapi "github.com/wins/jaz/backend/internal/httpapi/devices"
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

type routeDeps struct {
	fx.In

	Usage           usagecore.Service
	Devices         *deviceauth.Service            `optional:"true"`
	Config          serverconfig.Config            `optional:"true"`
	Browser         *browserworker.ExtensionBridge `optional:"true"`
	BrowserSettings *BrowserSettingsHandler        `optional:"true"`
	Connections     *connections.Service           `optional:"true"`
	ConnectionStart *connections.ConnectService    `optional:"true"`
	ConnectionOAuth *connections.OAuthService      `optional:"true"`
	ConnectionQR    *connections.QRService         `optional:"true"`
}

func NewRoutes(deps routeDeps) server.Routes {
	routes := usageRoutes(deps.Usage)
	routes = appendConnectionRoutes(routes, deps.Connections, deps.ConnectionStart, deps.ConnectionOAuth, deps.ConnectionQR)
	routes = appendDeviceRoutes(routes, deps.Devices, deps.Config)
	return appendBrowserRoutes(routes, deps.BrowserSettings, deps.Browser)
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

func appendConnectionRoutes(routes server.Routes, service *connections.Service, connect *connections.ConnectService, oauth *connections.OAuthService, qr *connections.QRService) server.Routes {
	if service == nil {
		return routes
	}
	handler := connectionsapi.NewPluginHandler(service)
	routes = append(routes,
		server.Route{Pattern: "GET /v1/connections/plugins", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "GET /v1/connections/plugins/{id}", Handler: httpHandlerFunc(handler.Get)},
		server.Route{Pattern: "DELETE /v1/connections/accounts/{id}", Handler: httpHandlerFunc(handler.Disconnect)},
	)
	if connect != nil || oauth != nil || qr != nil {
		connectHandler := connectionsapi.NewConnectHandler(connect, oauth, qr)
		if connect != nil {
			routes = append(routes, server.Route{Pattern: "POST /v1/connections/plugins/{id}/connect", Handler: httpHandlerFunc(connectHandler.Start)})
		}
		if qr != nil {
			routes = append(routes, server.Route{Pattern: "GET /v1/connections/qr/{id}", Handler: httpHandlerFunc(connectHandler.QRStatus)})
			routes = append(routes, server.Route{Pattern: "DELETE /v1/connections/qr/{id}", Handler: httpHandlerFunc(connectHandler.CloseQR)})
		}
		if oauth != nil {
			routes = append(routes, server.Route{Pattern: "GET /v1/connections/oauth/google/callback", Handler: httpHandlerFunc(connectHandler.GoogleCallback)})
		}
	}
	return routes
}

func appendDeviceRoutes(routes server.Routes, devices *deviceauth.Service, cfg serverconfig.Config) server.Routes {
	if devices == nil {
		return routes
	}
	handler := deviceapi.NewHandler(devices, cfg)
	return append(routes,
		server.Route{Pattern: "GET /v1/devices/connection-link", Handler: httpHandlerFunc(handler.ConnectionLink)},
		server.Route{Pattern: "GET /v1/devices", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "POST /v1/devices/register", Handler: httpHandlerFunc(handler.Register)},
		server.Route{Pattern: "POST /v1/devices/pairing-requests", Handler: httpHandlerFunc(handler.CreatePairing)},
		server.Route{Pattern: "/v1/devices/pairing-requests/", Handler: httpHandlerFunc(handler.Pairing)},
		server.Route{Pattern: "DELETE /v1/devices/{id}", Handler: httpHandlerFunc(handler.Revoke)},
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

type httpHandlerFunc func(http.ResponseWriter, *http.Request)

func (f httpHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}
