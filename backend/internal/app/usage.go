package app

import (
	"github.com/wins/jaz/backend/internal/deviceauth"
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
