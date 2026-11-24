package app

import (
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/computercontrol"
	feedcore "github.com/wins/jaz/backend/internal/feed"
	agentsessionsapi "github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	botsapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	canvasapi "github.com/wins/jaz/backend/internal/httpapi/canvas"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	computerapi "github.com/wins/jaz/backend/internal/httpapi/computer"
	feedapi "github.com/wins/jaz/backend/internal/httpapi/feed"
	filesystemapi "github.com/wins/jaz/backend/internal/httpapi/filesystem"
	hooksapi "github.com/wins/jaz/backend/internal/httpapi/hooks"
	m365copilotapi "github.com/wins/jaz/backend/internal/httpapi/m365copilot"
	mcpappsapi "github.com/wins/jaz/backend/internal/httpapi/mcpapps"
	mcpsearchapi "github.com/wins/jaz/backend/internal/httpapi/mcpsearch"
	mobilepushapi "github.com/wins/jaz/backend/internal/httpapi/mobilepush"
	modelcapabilitiesapi "github.com/wins/jaz/backend/internal/httpapi/modelcapabilities"
	pluginsapi "github.com/wins/jaz/backend/internal/httpapi/plugins"
	previewapi "github.com/wins/jaz/backend/internal/httpapi/preview"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	voiceapi "github.com/wins/jaz/backend/internal/httpapi/voice"
	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/machinebridge"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/mobilepush"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/storage"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"go.uber.org/fx"
)

type routeDeps struct {
	fx.In

	ACP              *acp.Manager
	Usage            usagecore.Service
	Feed             feedcore.Service
	ModelCatalog     *modelcatalog.Service `optional:"true"`
	Store            storage.Store
	Config           serverconfig.Config               `optional:"true"`
	Browser          *browsercontrol.ConfiguredBackend `optional:"true"`
	Computer         *computercontrol.DesktopBackend   `optional:"true"`
	ComputerSettings *ComputerSettingsHandler          `optional:"true"`
	BrowserSettings  *BrowserSettingsHandler           `optional:"true"`
	MCP              *mcpruntime.Manager               `optional:"true"`
	Plugins          *plugins.Service                  `optional:"true"`
	PluginSessions   plugins.SessionStore              `optional:"true"`
	PluginViews      *pluginsapi.ViewHandler
	Preview          *previewapi.Handler
	SessionMessages  *sessionsapi.MessagesHandler
	SessionOverview  *sessionsapi.OverviewHandler
	SessionGoal      *sessionsapi.GoalHandler
	Canvas           *canvasapi.Handler
	AgentSession     *agentsessionsapi.Handler
	Voice            *voiceapi.Handler
	VoiceRelay       *voiceapi.RelayHandler
	Push             *mobilepush.Service
	MicrosoftCopilot *m365copilotapi.Handler
	Cloud            *cloudsync.Service       `optional:"true"`
	MachineBridge    machinebridge.Controller `optional:"true"`
}

func NewRoutes(deps routeDeps) server.Routes {
	listHandler := sessionsapi.NewListHandler(deps.Store)
	messagesHandler := deps.SessionMessages
	usageService := deps.Usage
	feedService := deps.Feed
	if deps.Cloud != nil {
		listHandler.WithCloud(deps.Cloud)
		messagesHandler = messagesHandler.WithCloud(deps.Cloud)
		usageService = usageService.WithWorkspace(deps.Cloud.ActiveWorkspaceID)
		feedService = feedService.WithWorkspace(deps.Cloud.ActiveWorkspaceID)
	}
	routes := server.Routes{
		{Pattern: "GET /v1/acp/agents/{agent}/models", Handler: modelcapabilitiesapi.NewAgentModelsHandler(deps.ACP)},
		{Pattern: "GET " + m365copilot.OAuthCallbackPath, Handler: http.HandlerFunc(deps.MicrosoftCopilot.Callback)},
		{Pattern: "POST /v1/filesystem/dirs", Handler: http.HandlerFunc(filesystemapi.CreateDirectory)},
		{Pattern: "GET /v1/sessions", Handler: listHandler},
		{Pattern: "PUT /v1/sessions/{session}/agent/config", Handler: http.HandlerFunc(deps.AgentSession.SetConfig)},
		{Pattern: "PUT /v1/sessions/{session}/mcp/discovery", Handler: http.HandlerFunc(deps.AgentSession.SetMCPDiscovery)},
		{Pattern: "POST /v1/sessions/{session}/agent/tasks/{task}/stop", Handler: http.HandlerFunc(deps.AgentSession.StopTask)},
		{Pattern: "POST /v1/sessions/{session}/agent/input", Handler: http.HandlerFunc(deps.AgentSession.Input)},
		{Pattern: "GET /v1/sessions/{session}/messages", Handler: messagesHandler},
		{Pattern: "GET /v1/sessions/{session}/overview", Handler: deps.SessionOverview},
		{Pattern: "POST /v1/sessions/{session}/plugin-views", Handler: http.HandlerFunc(deps.PluginViews.Open)},
		{Pattern: "GET /v1/sessions/{session}/plugin-views/state", Handler: http.HandlerFunc(deps.PluginViews.State)},
		{Pattern: "PUT /v1/sessions/{session}/plugin-views/state", Handler: http.HandlerFunc(deps.PluginViews.SaveState)},
		{Pattern: "DELETE /v1/sessions/{session}/goal", Handler: deps.SessionGoal},
		{Pattern: "GET /v1/sessions/{session}/canvas", Handler: http.HandlerFunc(deps.Canvas.Get)},
		{Pattern: "PATCH /v1/sessions/{session}/canvas", Handler: http.HandlerFunc(deps.Canvas.Patch)},
		{Pattern: "PUT /v1/sessions/{session}/canvas/layout", Handler: http.HandlerFunc(deps.Canvas.PutLayout)},
		{Pattern: "POST /v1/sessions/{session}/canvas/files", Handler: http.HandlerFunc(deps.Canvas.PostFile)},
		{Pattern: "GET /v1/settings/voice", Handler: http.HandlerFunc(deps.Voice.Settings)},
		{Pattern: "PUT /v1/settings/voice", Handler: http.HandlerFunc(deps.Voice.Settings)},
		{Pattern: "POST /v1/voice/connect", Handler: http.HandlerFunc(deps.Voice.Connect)},
		{Pattern: "GET /v1/voice/desktop", Handler: http.HandlerFunc(deps.VoiceRelay.Desktop)},
		{Pattern: "GET /v1/voice/claude/status", Handler: http.HandlerFunc(deps.VoiceRelay.Status)},
		{Pattern: "GET /v1/sessions/{session}/voice/claude", Handler: http.HandlerFunc(deps.VoiceRelay.Call)},
		{Pattern: "POST /v1/sessions/{session}/voice/transcript", Handler: http.HandlerFunc(deps.Voice.Transcript)},
		{Pattern: "POST /v1/sessions/{session}/voice/diagnostics", Handler: http.HandlerFunc(deps.Voice.Diagnostics)},
	}
	routes = append(routes, usageRoutes(usageService)...)
	push := mobilepushapi.Handler{Service: deps.Push}
	routes = append(routes,
		server.Route{Pattern: "GET /v1/mobile/push", Handler: http.HandlerFunc(push.Status)},
		server.Route{Pattern: "PUT /v1/mobile/push", Handler: http.HandlerFunc(push.Register)},
		server.Route{Pattern: "DELETE /v1/mobile/push", Handler: http.HandlerFunc(push.Unregister)},
		server.Route{Pattern: "PUT /v1/mobile/activities/{activity}", Handler: http.HandlerFunc(push.RegisterActivity)},
		server.Route{Pattern: "DELETE /v1/mobile/activities/{activity}", Handler: http.HandlerFunc(push.Unregister)},
		server.Route{Pattern: "GET /v1/notifications/stream", Handler: http.HandlerFunc(push.Stream)},
	)
	routes = appendPluginRoutes(routes, deps.Plugins, deps.PluginSessions, deps.MCP, deps.Config, deps.Cloud)
	routes = append(routes, feedRoutes(feedService)...)
	routes = append(routes, modelCapabilityRoutes(deps.ModelCatalog)...)
	if deps.Computer != nil && deps.ComputerSettings != nil {
		routes = append(routes,
			server.Route{Pattern: "GET /v1/computer", Handler: deps.ComputerSettings},
			server.Route{Pattern: "PUT /v1/computer", Handler: deps.ComputerSettings},
			server.Route{Pattern: "GET /v1/sessions/{session}/computer", Handler: computerapi.DesktopHandler{Backend: deps.Computer, Store: deps.Store}},
		)
	}
	routes = appendBrowserRoutes(routes, deps.BrowserSettings, deps.Browser, deps.Store)
	routes = appendMCPRoutes(routes, deps.MCP)
	routes = appendCloudRoutes(routes, deps.Cloud, deps.MachineBridge, deps.ACP)
	return append(routes, server.Route{Pattern: "/v1/preview/", Handler: deps.Preview})
}

func appendCloudRoutes(routes server.Routes, cloud *cloudsync.Service, bridge machinebridge.Controller, acpMgr *acp.Manager) server.Routes {
	if cloud == nil {
		return routes
	}
	h := cloudapi.NewHandler(cloud).WithMachineBridge(bridge)
	acpProfiles := cloudapi.NewACPProfileHandler(cloud, acpMgr)
	return append(routes,
		server.Route{Pattern: "GET /v1/cloud/status", Handler: httpHandlerFunc(h.Status)},
		server.Route{Pattern: "GET /v1/cloud/machine-bridge", Handler: httpHandlerFunc(h.MachineBridgeStatus)},
		server.Route{Pattern: "PUT /v1/cloud/machine-bridge", Handler: httpHandlerFunc(h.UpdateMachineBridge)},
		server.Route{Pattern: "POST /v1/cloud/connect", Handler: httpHandlerFunc(h.Connect)},
		server.Route{Pattern: "POST /v1/cloud/switch", Handler: httpHandlerFunc(h.Switch)},
		server.Route{Pattern: "POST /v1/cloud/disconnect", Handler: httpHandlerFunc(h.Disconnect)},
		server.Route{Pattern: "POST /v1/cloud/sync", Handler: httpHandlerFunc(h.Sync)},
		server.Route{Pattern: "POST /v1/cloud/acp-profiles/{agent}/sync-local", Handler: httpHandlerFunc(acpProfiles.SyncLocal)},
		server.Route{Pattern: "GET /v1/acp/profiles", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "PUT /v1/acp/profiles/{agent}", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "DELETE /v1/acp/profiles/{agent}", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "POST /v1/acp/profiles/{agent}/refresh", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "POST /v1/acp/profiles/{agent}/login", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "GET /v1/acp/profiles/{agent}/login/{id}", Handler: httpHandlerFunc(acpProfiles.Proxy)},
		server.Route{Pattern: "GET /v1/models", Handler: httpHandlerFunc(h.ListModels)},
		server.Route{Pattern: "GET /v1/machines", Handler: httpHandlerFunc(h.ListMachines)},
		server.Route{Pattern: "GET /v1/members", Handler: httpHandlerFunc(h.ListMembers)},
		server.Route{Pattern: "POST /v1/members", Handler: httpHandlerFunc(h.CreateMember)},
		server.Route{Pattern: "PATCH /v1/members/{id}", Handler: httpHandlerFunc(h.UpdateMember)},
		server.Route{Pattern: "DELETE /v1/members/{id}", Handler: httpHandlerFunc(h.DeleteMember)},
		server.Route{Pattern: "GET /v1/spaces", Handler: httpHandlerFunc(h.ListSpaces)},
		server.Route{Pattern: "POST /v1/spaces", Handler: httpHandlerFunc(h.CreateSpace)},
		server.Route{Pattern: "PATCH /v1/spaces/{id}", Handler: httpHandlerFunc(h.PatchSpace)},
		server.Route{Pattern: "POST /v1/spaces/{id}/members", Handler: httpHandlerFunc(h.AddSpaceMember)},
		server.Route{Pattern: "GET /v1/instructions/{scope}/{scope_id}/{kind}", Handler: httpHandlerFunc(h.GetInstruction)},
		server.Route{Pattern: "PUT /v1/instructions/{scope}/{scope_id}/{kind}", Handler: httpHandlerFunc(h.PutInstruction)},
		server.Route{Pattern: "GET /v1/usage/summary", Handler: httpHandlerFunc(h.UsageSummary)},
		server.Route{Pattern: "POST /v1/threads/{id}/turns", Handler: httpHandlerFunc(h.CreateTurn)},
		server.Route{Pattern: "POST /v1/threads/{id}/cancel", Handler: httpHandlerFunc(h.CancelThread)},
		server.Route{Pattern: "POST /v1/threads/{id}/steer", Handler: httpHandlerFunc(h.SteerThread)},
		server.Route{Pattern: "PUT /v1/threads/{id}/user-state", Handler: httpHandlerFunc(h.UpdateThreadUserState)},
		server.Route{Pattern: "POST /v1/runs/{run_id}/uploads/presign", Handler: httpHandlerFunc(h.PresignUpload)},
		server.Route{Pattern: "POST /v1/runs/{run_id}/uploads/commit", Handler: httpHandlerFunc(h.CommitUpload)},
		server.Route{Pattern: "GET /v1/runs/{run_id}/files/{path...}", Handler: httpHandlerFunc(h.ProxyRunFile)},
		server.Route{Pattern: "GET /v1/user-skills", Handler: httpHandlerFunc(h.Proxy)},
		server.Route{Pattern: "PUT /v1/user-skills/{name}", Handler: httpHandlerFunc(h.Proxy)},
		server.Route{Pattern: "DELETE /v1/user-skills/{name}", Handler: httpHandlerFunc(h.Proxy)},
		server.Route{Pattern: "GET /v1/connectors", Handler: httpHandlerFunc(h.Proxy)},
		server.Route{Pattern: "DELETE /v1/connectors/{id}", Handler: httpHandlerFunc(h.Proxy)},
	)
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

func appendMCPRoutes(routes server.Routes, mcp *mcpruntime.Manager) server.Routes {
	if mcp == nil {
		return routes
	}
	handler := mcpappsapi.NewHandler(mcp)
	return append(routes,
		server.Route{Pattern: "GET /v1/mcp/apps", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "GET /v1/mcp/apps/{server}/resource", Handler: httpHandlerFunc(handler.Resource)},
		server.Route{Pattern: "POST /v1/mcp/apps/{server}/tools/call", Handler: httpHandlerFunc(handler.CallTool)},
		server.Route{Pattern: "GET /v1/search/connections", Handler: mcpsearchapi.NewHandler(mcp)},
	)
}

func feedRoutes(feed feedcore.Service) server.Routes {
	handler := feedapi.NewHandler(feed)
	return server.Routes{
		{
			Pattern: "GET /v1/feed",
			Handler: httpHandlerFunc(handler.List),
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

func appendPluginRoutes(routes server.Routes, service *plugins.Service, sessions plugins.SessionStore, mcp pluginsapi.MCPRuntime, cfg serverconfig.Config, cloud *cloudsync.Service) server.Routes {
	if service == nil {
		return routes
	}
	handler := pluginsapi.NewHandler(service, sessions, mcp, cfg.PublicBaseURL())
	if cloud != nil {
		handler.WithCloud(cloud)
	}
	return append(routes,
		server.Route{Pattern: "GET /v1/plugins", Handler: httpHandlerFunc(handler.List)},
		server.Route{Pattern: "GET /v1/plugins/sources", Handler: httpHandlerFunc(handler.ListSources)},
		server.Route{Pattern: "POST /v1/plugins/sources", Handler: httpHandlerFunc(handler.AddSource)},
		server.Route{Pattern: "DELETE /v1/plugins/sources/{id}", Handler: httpHandlerFunc(handler.RemoveSource)},
		server.Route{Pattern: "POST /v1/plugins/sources/refresh", Handler: httpHandlerFunc(handler.RefreshSources)},
		server.Route{Pattern: "POST /v1/plugins/install", Handler: httpHandlerFunc(handler.Install)},
		server.Route{Pattern: "GET " + plugins.OAuthCallbackPath, Handler: httpHandlerFunc(handler.OAuthCallback)},
		server.Route{Pattern: "DELETE /v1/plugins/accounts/{id}", Handler: httpHandlerFunc(handler.DisconnectAccount)},
		server.Route{Pattern: "GET /v1/plugins/qr/{id}/status", Handler: httpHandlerFunc(handler.QRStatus)},
		server.Route{Pattern: "POST /v1/plugins/qr/{id}/password", Handler: httpHandlerFunc(handler.QRPassword)},
		server.Route{Pattern: "DELETE /v1/plugins/qr/{id}", Handler: httpHandlerFunc(handler.CloseQR)},
		server.Route{Pattern: "GET /v1/plugins/{plugin}", Handler: httpHandlerFunc(handler.Get)},
		server.Route{Pattern: "PUT /v1/plugins/{plugin}/enabled", Handler: httpHandlerFunc(handler.SetEnabled)},
		server.Route{Pattern: "PUT /v1/plugins/{plugin}/config", Handler: httpHandlerFunc(handler.SaveConfig)},
		server.Route{Pattern: "POST /v1/plugins/{plugin}/connect", Handler: httpHandlerFunc(handler.Connect)},
		server.Route{Pattern: "GET /v1/plugins/{plugin}/page", Handler: httpHandlerFunc(handler.Page)},
		server.Route{Pattern: "POST /v1/plugins/{plugin}/update", Handler: httpHandlerFunc(handler.Update)},
		server.Route{Pattern: "DELETE /v1/plugins/{plugin}", Handler: httpHandlerFunc(handler.Remove)},
	)
}

func appendBrowserRoutes(routes server.Routes, settings *BrowserSettingsHandler, backend *browsercontrol.ConfiguredBackend, store storage.Store) server.Routes {
	if settings != nil {
		routes = append(routes,
			server.Route{Pattern: "GET /v1/browser", Handler: settings},
			server.Route{Pattern: "PUT /v1/browser", Handler: settings},
		)
	}
	if backend != nil {
		routes = append(routes,
			server.Route{Pattern: "GET /v1/browser/desktop", Handler: browserapi.DesktopHandler{Backend: backend.DesktopBackend, Store: store}},
			server.Route{Pattern: "GET /v1/sessions/{session}/browser", Handler: browserapi.DesktopHandler{Backend: backend.DesktopBackend, Store: store}},
			server.Route{Pattern: "POST /v1/sessions/{session}/browser", Handler: browserapi.DesktopHandler{Backend: backend.DesktopBackend, Store: store}},
		)
	}
	return routes
}

func BotRoutes(handler *botsapi.Handler) server.Routes {
	return server.Routes{
		{Pattern: "GET /v1/bots/events", Handler: httpHandlerFunc(handler.Events)},
		{Pattern: "GET /v1/bots", Handler: httpHandlerFunc(handler.List)},
		{Pattern: "POST /v1/bots", Handler: httpHandlerFunc(handler.Create)},
		{Pattern: "POST /v1/bots/groups", Handler: httpHandlerFunc(handler.CreateGroup)},
		{Pattern: "PUT /v1/bots/pins", Handler: httpHandlerFunc(handler.Pin)},
		{Pattern: "GET /v1/bots/{bot}", Handler: httpHandlerFunc(handler.Get)},
		{Pattern: "PATCH /v1/bots/{bot}", Handler: httpHandlerFunc(handler.Update)},
		{Pattern: "DELETE /v1/bots/{bot}", Handler: httpHandlerFunc(handler.Delete)},
		{Pattern: "POST /v1/bots/{bot}/messages", Handler: httpHandlerFunc(handler.Post)},
		{Pattern: "GET /v1/bots/{bot}/activity", Handler: httpHandlerFunc(handler.Activity)},
		{Pattern: "GET /v1/bots/{bot}/files", Handler: httpHandlerFunc(handler.Files)},
		{Pattern: "PUT /v1/bots/{bot}/files/{name}", Handler: httpHandlerFunc(handler.SaveFile)},
	}
}

// HookRoute serves routine webhooks without the device key: each routine
// authenticates its callers with its own secret.
func HookRoute(handler *hooksapi.Handler) server.PublicRoute {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/hooks/{routine}", handler.Webhook)
	return server.PublicRoute{
		Match: func(r *http.Request) bool {
			return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/hooks/")
		},
		Handler: mux,
	}
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
