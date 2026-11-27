package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpadapter"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/app"
	appscore "github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/computercontrol"
	configloader "github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/connections"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/coordinator"
	botsapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	hooksapi "github.com/wins/jaz/backend/internal/httpapi/hooks"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/jaztools"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/managedtool"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeauth"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/terminal"
	"github.com/wins/jaz/backend/internal/threads"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/voice"
	"github.com/wins/jaz/backend/internal/widgets"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func runServe(args []string) error {
	fxApp := fx.New(serveOptions(args)...)
	if err := fxApp.Err(); err != nil {
		return conciseError(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := fxApp.Start(ctx); err != nil {
		return conciseError(err)
	}
	shutdown := <-fxApp.Wait()
	fmt.Fprintf(os.Stderr, "jaz server stopping signal=%v exit_code=%d\n", shutdown.Signal, shutdown.ExitCode)
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return conciseError(fxApp.Stop(ctx))
}

func serveOptions(args []string) []fx.Option {
	return []fx.Option{
		fx.StopTimeout(15 * time.Second),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Supply(serveArgs{Args: args}),
		fx.Provide(
			newLogger,
			loadConfig,
			parseServeConfig,
			serverconfig.NewURLs,
			app.NewACPAgentCatalog,
			app.NewRuntimeLayout,
			app.NewStore,
			cloudsync.New,
			app.NewRuntimeAuthKey,
			app.NewWorkspace,
			app.NewMemory,
			connections.NewCatalog,
			app.NewConnectionMCPConnector,
			app.NewConnectionService,
			app.NewOAuthBroker,
			app.NewConnectionOAuthService,
			app.NewIntegrationRawWriter,
			app.NewMemorySourceQueue,
			app.NewSourceProjectionQueue,
			integrationingest.NewObservers,
			app.NewIntegrationMaterializingWriter,
			app.NewSourceProjectionRunner,
			app.NewWhatsAppProvider,
			app.NewTelegramProvider,
			app.NewConnectionQRService,
			app.NewConnectionConnectService,
			app.NewCalendarMCPTools,
			app.NewGmailMCPTools,
			app.NewWhatsAppMCPTools,
			app.NewTelegramMCPTools,
			app.NewGmailSyncer,
			newMemoryService,
			fx.Annotate(visualize.NewService, fx.From(new(*sqlitestore.Store), new(*sqlitestore.Store), new(*sessionevents.Bus))),
			jaztools.New,
			terminal.New,
			app.NewACPMCPServerReader,
			exectool.NewCommandManager,
			app.NewInstructionsStore,
			app.NewPromptBuilder,
			app.NewACPAgentConfigSource,
			app.NewACPAdapterManager,
			app.NewManagedToolManager,
			app.NewMicrosoftCopilotAuth,
			app.NewACPConfig,
			acp.NewManager,
			sessionlock.New,
			sessionevents.New,
			app.NewThreadService,
			app.NewWidgetService,
			app.NewWidgetSessionPublisher,
			app.NewToolRegistry,
			computercontrol.NewDesktopBackend,
			app.NewComputerSettingsHandler,
			fx.Annotate(browsercontrol.NewConfiguredBackend, fx.From(new(*sqlitestore.Store))),
			app.NewBrowserSettingsHandler,
			app.NewMCPManager,
			app.NewProviderSource,
			app.NewModelCatalog,
			app.NewProvider,
			app.NewVoice,
			app.NewAgent,
		),
		app.HTTPModule(),
		fx.Invoke(
			app.StartMobilePush,
			app.CloseMemory,
			app.StartStorageMaintenance,
			app.ConfigureMemoryDreamRunner,
			app.ConfigureMemorySearch,
			app.ConfigureComputerTools,
			app.CloseComputerBackend,
			app.ConfigureBrowserTools,
			app.ConfigureVaultTools,
			app.CloseBrowserBackend,
			app.StartMemoryScheduler,
			app.StartSourceProjectionWorker,
			app.StartPluginRefresh,
			app.StartGmailSync,
			app.StartModelCatalogWarmup,
			app.DeclareMCPServers,
			app.StartCloudSync,
			startServer,
		),
	}
}

type serveArgs struct {
	Args []string
}

// Log level comes from JAZ_LOG (debug, info, warn, error); defaults to info.
func newLogger() *log.Logger {
	level := log.InfoLevel
	if raw := os.Getenv("JAZ_LOG"); raw != "" {
		if parsed, err := log.ParseLevel(raw); err == nil {
			level = parsed
		}
	}
	return log.NewWithOptions(os.Stderr, log.Options{ReportTimestamp: true, Level: level})
}

type config struct {
	fx.Out

	Jaz        app.Config
	MCPClients map[string]mcpconfig.OAuthConfig
}

func loadConfig() (config, error) {
	loaded, err := configloader.Load()
	if err != nil {
		return config{}, err
	}
	return config{Jaz: loaded.Jaz, MCPClients: loaded.Jaz.Connections.MCP}, nil
}

func parseServeConfig(args serveArgs) (serverconfig.Config, error) {
	fs := flag.NewFlagSet("jaz", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:5299", "HTTP listen address")
	publicURL := fs.String("public-url", "", "URL shown to Jaz clients")
	allowedOrigins := fs.String("allowed-origins", "", "comma-separated browser client origins")
	previewURLTemplate := fs.String("preview-url-template", "", "isolated preview origin URL containing {id}")
	if err := fs.Parse(args.Args); err != nil {
		return serverconfig.Config{}, err
	}
	cfg := serverconfig.New(*addr, *publicURL)
	for _, origin := range strings.Split(*allowedOrigins, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host == "" || parsed.Path != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
				return serverconfig.Config{}, fmt.Errorf("invalid allowed browser origin")
			}
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, origin)
		}
	}
	cfg.PreviewURLTemplate = strings.TrimSpace(*previewURLTemplate)
	return cfg, nil
}

func newMemoryService(cfg app.Config, memory *jazmem.Memory, store *sqlitestore.Store, logger *log.Logger, urls serverconfig.URLs) *memoryservice.Service {
	scheduler := memoryservice.NewScheduler(memory, cfg.Memory.Scheduler, logger)
	return memoryservice.New(memory, store, scheduler, urls.JazmemMCP)
}

func conciseError(err error) error {
	if err == nil {
		return nil
	}
	for unwrapped := errors.Unwrap(err); unwrapped != nil; unwrapped = errors.Unwrap(err) {
		err = unwrapped
	}
	text := err.Error()
	if i := strings.LastIndex(text, "): "); i >= 0 {
		text = text[i+3:]
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return fmt.Errorf("%s", line)
		}
	}
	return err
}

func startServer(
	lc fx.Lifecycle,
	a *agent.Agent,
	layout runtimefiles.Layout,
	store *sqlitestore.Store,
	cloud *cloudsync.Service,
	manager *acp.Manager,
	adapters *acpadapter.Manager,
	managedTools *managedtool.Manager,
	msCopilot *m365copilot.AuthService,
	locks *sessionlock.Locks,
	events *sessionevents.Bus,
	inst instructions.Store,
	prompts *coordinator.Builder,
	installed *plugins.Store,
	mcpManager *mcpruntime.Manager,
	workspace app.Workspace,
	stt voice.STT,
	tts voice.TTS,
	modelProviderRuntime provider.Provider,
	providerSource provider.Source,
	modelCatalog *modelcatalog.Service,
	logger *log.Logger,
	cfg app.Config,
	catalog acp.AgentCatalog,
	serverConfig serverconfig.Config,
	widgetService *widgets.Service,
	widgetPublisher *widgets.SessionPublisher,
	memory *memoryservice.Service,
	sourceProjectionQueue app.SourceProjectionQueue,
	memorySourceQueue app.MemorySourceQueue,
	jazTools *jaztools.Service,
	appsService *appscore.Service,
	terminals *terminal.Manager,
	threadService *threads.Service,
	runtimeAuthKey app.RuntimeAuthKey,
	routes server.Routes,
	publicRoutes server.PublicRoutes,
	recordObservers *integrationingest.Observers,
	ingestWriter integrationingest.MaterializingWriter,
) error {
	authKey := string(runtimeAuthKey)
	handler := &server.Server{
		Agent:                 a,
		Store:                 store,
		Cloud:                 cloud,
		Routes:                routes,
		PublicRoutes:          publicRoutes,
		ACP:                   manager,
		ACPAdapters:           adapters,
		ManagedTools:          managedTools,
		MicrosoftCopilot:      msCopilot,
		MCP:                   mcpManager,
		Locks:                 locks,
		Events:                events,
		Threads:               threadService,
		STT:                   stt,
		TTS:                   tts,
		ModelProviderRuntime:  reloadableProvider(modelProviderRuntime),
		ModelCatalog:          modelCatalog,
		Providers:             providerSource,
		AgentCatalog:          catalog,
		AuthKey:               authKey,
		ServerConfig:          serverConfig,
		Instructions:          inst,
		Prompts:               prompts,
		PluginsDir:            installed.ActivePluginsDir,
		Root:                  store.RootDir(),
		Workspace:             string(workspace),
		Log:                   logger.WithPrefix("server"),
		Memory:                memory,
		JazTools:              jazTools,
		SourceProjectionQueue: sourceProjectionQueue.Queue,
		MemorySourceQueue:     memorySourceQueue.Queue,
		Terminal:              terminals,
	}
	app.ConnectACPCompletion(manager, a, store, locks, events, prompts, logger)
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			terminals.Close()
			return nil
		},
	})
	loopRunner := server.NewLoopRunner(handler)
	loopMemoryPaths := loops.NewMemoryPaths(loops.AutomationsDir(store.RootDir()))
	loopService := loops.NewService(store, loopRunner, logger,
		loops.WithMemoryPaths(loopMemoryPaths),
		// Board assignment is the widget enablement: no boards, no section.
		loops.WithPromptExtras(bots.RoutineInstructions(store), widgetService.LoopPromptExtra),
		loops.WithArtifactSurface(widgetService.LoopArtifactSurface),
		loops.WithRunCheck(widgetService.RequireRunPublished),
		loops.WithModelCheck(loopRunner.CheckModel),
		loops.WithWorkspace(cloud.ActiveWorkspaceID),
	)
	botService := bots.NewService(store, layout.Bots, inst, manager, handler, handler, loopService, events, logger).WithWorkspace(cloud.ActiveWorkspaceID)
	loopService.Notice = botService.Notice
	mcpManager.AppVisible = botService.AppVisible
	jazTools.SetLoops(loopService,
		loops.WithBoards(widgetService.LoopBoards()),
		loops.WithAgentNames(manager.Agents),
		loops.WithEvents(store, events),
		loops.WithOwner(botService.RoutineOwner),
		loops.WithScope(botService.BotOf),
	)
	jazTools.SetBots(bots.NewMCPTools(botService))
	jazTools.SetApps(appsService)
	handler.Routes = append(handler.Routes, app.BotRoutes(botsapi.NewHandler(botService, events).WithCloud(cloud))...)
	handler.PublicRoutes = append(handler.PublicRoutes, app.HookRoute(hooksapi.NewHandler(loopService)))
	handler.RoutineOwner = botService.RoutineOwner
	recordObservers.Add(loopService.HandleIncoming)
	jazTools.SetThreads(threadService)
	jazTools.SetAgents(manager)
	handler.Loops = loopService
	handler.Widgets = widgetService
	manager.PublishWidget = func(req acp.WidgetPublishRequest) (acp.WidgetPublishResult, error) {
		widget, warnings, err := widgetPublisher.PublishForSession(req.SessionID, widgets.PublishInput{
			Title:    req.Title,
			SizeHint: req.SizeHint,
			HTML:     req.HTML,
		})
		if err != nil {
			return acp.WidgetPublishResult{}, err
		}
		return acp.WidgetPublishResult{
			WidgetID: widget.ID,
			Title:    widget.Title,
			Version:  widget.CurrentVersion,
			SizeHint: widget.SizeHint,
			Warnings: warnings,
		}, nil
	}
	manager.TurnEnding = botService.TurnEnding
	manager.TurnFinished = func(ctx context.Context, job acp.Job) {
		handler.HandleACPTurnFinished(ctx, job)
		botService.TurnEnded(job.ID)
	}
	manager.ThreadStarted = handler.TitleThread
	srv := &http.Server{
		Addr:              serverConfig.Addr,
		Handler:           handler.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	var stopLoops context.CancelFunc
	// The idle stopper finishes any stop it began before the manager closes.
	idleStopped := make(chan struct{})
	worktreesStopped := make(chan struct{})
	// Routines start once restart recovery has resumed the turns and queues
	// that hold their runs, so a run goes on or catches up, never both.
	recovered := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			fmt.Printf("jaz server listening on %s\n", serverconfig.DisplayAddr(serverConfig.Addr))
			fmt.Printf("client: %s\n", serverconfig.ClientBaseURL(serverConfig))
			fmt.Printf("client key: %s\n", runtimeauth.Path(store.RootDir()))
			fmt.Printf("root: %s\n", store.RootDir())
			fmt.Printf("workspace: %s\n", workspace)
			if err := loopMemoryPaths.EnsureDir(); err != nil {
				logger.WithPrefix("loops").Warn("loop memory directory unavailable", "path", loopMemoryPaths.Dir(), "error", err)
			}
			if err := loopService.EnsureMemoryPaths(); err != nil {
				return err
			}
			// Events and webhooks can arrive before the scheduler's first tick.
			if err := loopService.GuardAway(); err != nil {
				logger.WithPrefix("loops").Error("away guard failed", "error", err)
			}
			listener, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			go func() {
				if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
					fmt.Fprintln(os.Stderr, "serve:", err)
				}
			}()
			handler.WarmSlowACPAuth()
			if err := botService.Declare(ctx, cfg.Bots); err != nil {
				_ = listener.Close()
				_ = srv.Close()
				manager.Close()
				return err
			}
			// Without an agent to run on, as before setup, Jaz tries again at
			// the next start.
			if err := botService.EnsureMain(ctx); err != nil {
				logger.WithPrefix("bots").Error("creating the main bot failed", "error", err)
			}
			loopCtx, cancelLoops := context.WithCancel(context.Background())
			stopLoops = cancelLoops
			go func() {
				defer close(idleStopped)
				manager.StopIdleProcesses(loopCtx, acp.DefaultIdleTimeouts)
			}()
			slackSync := &integrationingest.SlackSyncer{
				Store:      store,
				Writer:     ingestWriter,
				Wanted:     func() bool { return loopService.Watching(loops.TriggerSlack) },
				APIBaseURL: slackconnector.APIBaseURL,
			}
			go slackSync.Run(loopCtx, time.Minute, func(err error) {
				logger.WithPrefix("slack-sync").Warn("slack sync failed", "error", err)
			})
			go func() {
				// Ownerless loops get their bots before the scheduler can run them.
				if err := botService.AdoptLoops(loopCtx); err != nil {
					logger.WithPrefix("bots").Error("adopting loops failed", "error", err)
				}
				select {
				case <-recovered:
				case <-loopCtx.Done():
					return
				}
				if err := loops.StartScheduler(loopCtx, loopService, 30*time.Second, handler.ResumeLoopRuns); err != nil && loopCtx.Err() == nil {
					logger.WithPrefix("loops").Error("scheduler stopped", "error", err)
				}
			}()
			go func() {
				defer close(worktreesStopped)
				select {
				case <-recovered:
					app.RunWorktreeMaintenance(loopCtx, store, handler.Workspace, handler.PruneManagedWorktrees, logger.WithPrefix("worktrees"))
				case <-loopCtx.Done():
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if stopLoops != nil {
				stopLoops()
				<-idleStopped
				<-worktreesStopped
			}
			manager.Close()
			return stopHTTPServer(ctx, srv)
		},
	})
	app.StartMCPManager(lc, mcpManager, manager, logger)
	app.StartBotTaskWatch(lc, botService, recovered)
	app.StartSessionRecovery(lc, store, handler, locks, events, logger, func(context.Context) error {
		defer close(recovered)
		return errors.Join(handler.ResumeQueuedTurns(), botService.WatchAll())
	})
	return nil
}

func reloadableProvider(p provider.Provider) provider.ReloadableProvider {
	control, _ := p.(provider.ReloadableProvider)
	return control
}

func stopHTTPServer(ctx context.Context, srv *http.Server) error {
	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(ctx) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-time.After(750 * time.Millisecond):
		return closeHTTPServer(srv)
	case <-ctx.Done():
		return closeHTTPServer(srv)
	}
}

func closeHTTPServer(srv *http.Server) error {
	if err := srv.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
