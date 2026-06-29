package app

import (
	"github.com/wins/jaz/backend/internal/deviceauth"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

func UsageModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		feedcore.NewService,
		NewRoutes,
	)
}

func NewDeviceAuth(store *sqlitestore.Store) *deviceauth.Service {
	return deviceauth.New(store)
}
