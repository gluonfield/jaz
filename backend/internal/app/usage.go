package app

import (
	"net/http"

	"github.com/wins/jaz/backend/internal/deviceauth"
	deviceapi "github.com/wins/jaz/backend/internal/httpapi/devices"
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

func UsageModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		NewRoutes,
	)
}

func NewDeviceAuth(store *sqlitestore.Store) *deviceauth.Service {
	return deviceauth.New(store)
}

type routeDeps struct {
	fx.In

	Usage   usagecore.Service
	Devices *deviceauth.Service `optional:"true"`
	Config  serverconfig.Config `optional:"true"`
}

func NewRoutes(deps routeDeps) server.Routes {
	routes := server.Routes{
		{
			Pattern: "GET /v1/usage/daily",
			Handler: usageapi.NewDailyHandler(deps.Usage),
		},
		{
			Pattern: "GET /v1/usage/models",
			Handler: usageapi.NewModelsHandler(deps.Usage),
		},
	}
	if deps.Devices == nil {
		return routes
	}
	deviceHandler := deviceapi.NewHandler(deps.Devices, deps.Config)
	return append(routes,
		server.Route{Pattern: "GET /v1/devices/connection-link", Handler: httpHandlerFunc(deviceHandler.ConnectionLink)},
		server.Route{Pattern: "GET /v1/devices", Handler: httpHandlerFunc(deviceHandler.List)},
		server.Route{Pattern: "POST /v1/devices/register", Handler: httpHandlerFunc(deviceHandler.Register)},
		server.Route{Pattern: "POST /v1/devices/pairing-requests", Handler: httpHandlerFunc(deviceHandler.CreatePairing)},
		server.Route{Pattern: "/v1/devices/pairing-requests/", Handler: httpHandlerFunc(deviceHandler.Pairing)},
		server.Route{Pattern: "DELETE /v1/devices/{id}", Handler: httpHandlerFunc(deviceHandler.Revoke)},
	)
}

type httpHandlerFunc func(http.ResponseWriter, *http.Request)

func (f httpHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}
