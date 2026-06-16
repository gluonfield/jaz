package app

import (
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	"github.com/wins/jaz/backend/internal/server"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

func UsageModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		NewUsageRoutes,
	)
}

func NewUsageRoutes(service usagecore.Service) server.Routes {
	return server.Routes{{
		Pattern: "GET /v1/usage/daily",
		Handler: usageapi.NewDailyHandler(service),
	}}
}
