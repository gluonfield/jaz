package app

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/deviceauth"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	agentsessionsapi "github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	voiceapi "github.com/wins/jaz/backend/internal/httpapi/voice"
	"github.com/wins/jaz/backend/internal/sessionoverview"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"github.com/wins/jaz/backend/internal/voice/live"
	"go.uber.org/fx"
)

func HTTPModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		feedcore.NewService,
		NewVoiceCredentials,
		fx.Annotate(live.NewService, fx.From(new(*sqlitestore.Store))),
		fx.Annotate(live.NewTranscript, fx.From(new(*sqlitestore.Store))),
		voiceapi.NewHandler,
		previewapi.NewHandler,
		fx.Annotate(
			transcript.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager)),
		),
		fx.Annotate(
			sessionoverview.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager)),
		),
		fx.Annotate(agentsessionsapi.NewHandler, fx.From(new(*acp.Manager))),
		sessionsapi.NewMessagesHandler,
		sessionsapi.NewOverviewHandler,
		NewRoutes,
		NewPublicRoutes,
	)
}

func NewDeviceAuth(store *sqlitestore.Store) *deviceauth.Service {
	return deviceauth.New(store)
}
