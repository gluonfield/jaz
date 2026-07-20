package app

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/deviceauth"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/sessionoverview"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

func HTTPModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		feedcore.NewService,
		previewapi.NewHandler,
		fx.Annotate(
			transcript.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager)),
		),
		fx.Annotate(
			sessionoverview.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager)),
		),
		sessionsapi.NewMessagesHandler,
		sessionsapi.NewOverviewHandler,
		NewRoutes,
		NewPublicRoutes,
	)
}

func NewDeviceAuth(store *sqlitestore.Store) *deviceauth.Service {
	return deviceauth.New(store)
}
