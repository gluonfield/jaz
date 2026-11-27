package app

import (
	"github.com/wins/jaz/backend/internal/acp"
	appscore "github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/connections"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	agentsessionsapi "github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	appsapi "github.com/wins/jaz/backend/internal/httpapi/apps"
	canvasapi "github.com/wins/jaz/backend/internal/httpapi/canvas"
	m365copilotapi "github.com/wins/jaz/backend/internal/httpapi/m365copilot"
	pluginsapi "github.com/wins/jaz/backend/internal/httpapi/plugins"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	voiceapi "github.com/wins/jaz/backend/internal/httpapi/voice"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionoverview"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/voice/live"
	"go.uber.org/fx"
)

type appsServiceDeps struct {
	fx.In

	Store     *sqlitestore.Store
	Provider  provider.Provider   `optional:"true"`
	MCP       *mcpruntime.Manager `optional:"true"`
	Layout    runtimefiles.Layout
	Workspace Workspace
}

func NewAppsService(deps appsServiceDeps) *appscore.Service {
	var bridge appscore.ToolBridge
	if deps.MCP != nil {
		bridge = deps.MCP
	}
	return appscore.NewService(deps.Store, deps.Store, deps.Provider, bridge, deps.Layout.Root, string(deps.Workspace))
}

type appsHandlerDeps struct {
	fx.In

	Service *appscore.Service
	Config  serverconfig.Config `optional:"true"`
	Cloud   *cloudsync.Service  `optional:"true"`
}

func NewAppsHandler(deps appsHandlerDeps) *appsapi.Handler {
	h := appsapi.NewHandler(deps.Service, deps.Config.PublicBaseURL())
	if deps.Cloud != nil {
		h.WithCloud(deps.Cloud)
	}
	return h
}

type pluginStoreDeps struct {
	fx.In

	Layout  runtimefiles.Layout
	State   storage.PluginStore
	Catalog *connections.Catalog
	Cloud   *cloudsync.Service `optional:"true"`
}

func NewPluginStore(deps pluginStoreDeps) *plugins.Store {
	store := plugins.NewStore(deps.Layout, deps.State, deps.Catalog)
	if deps.Cloud != nil {
		store.WithWorkspace(deps.Cloud.ActiveWorkspaceID)
	}
	return store
}

func HTTPModule() fx.Option {
	return fx.Provide(
		usagecore.NewService,
		feedcore.NewService,
		NewVoiceCredentials,
		NewMobilePush,
		fx.Annotate(live.NewService, fx.From(new(*sqlitestore.Store))),
		fx.Annotate(live.NewTranscript, fx.From(new(*sqlitestore.Store))),
		voiceapi.NewHandler,
		live.NewDesktopRelay,
		fx.Annotate(voiceapi.NewRelayHandler, fx.From(new(*sqlitestore.Store))),
		previewapi.NewHandler,
		fx.Annotate(
			transcript.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager)),
		),
		fx.Annotate(
			sessionoverview.NewService,
			fx.From(new(*sqlitestore.Store), new(*acp.Manager), new(*plugins.Store)),
		),
		fx.Annotate(agentsessionsapi.NewHandler, fx.From(new(*acp.Manager))),
		sessionsapi.NewMessagesHandler,
		sessionsapi.NewOverviewHandler,
		fx.Annotate(sessionsapi.NewGoalHandler, fx.From(new(*acp.Manager))),
		canvas.NewService,
		NewPluginStore,
		plugins.NewService,
		fx.Annotate(plugins.NewViewService, fx.From(new(*sqlitestore.Store), new(*visualize.Service))),
		pluginsapi.NewViewHandler,
		fx.Annotate(canvasapi.NewHandler, fx.From(new(*sqlitestore.Store))),
		m365copilotapi.NewHandler,
		NewAppsService,
		NewAppsHandler,
		NewMachineBridge,
		NewRoutes,
		NewPublicRoutes,
	)
}
