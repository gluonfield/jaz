package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/app"
	configloader "github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/jaztools"
	"github.com/wins/jaz/backend/internal/loops"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeauth"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/terminal"
	"github.com/wins/jaz/backend/internal/threads"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"github.com/wins/jaz/backend/internal/voice"
	"github.com/wins/jaz/backend/internal/widgets"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func runServe(args []string) error {
	fxApp := fx.New(
		fx.StopTimeout(15*time.Second),
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
			sqlitestore.NewSearchQueries,
			app.NewWorkspace,
			app.NewMemory,
			app.NewDeviceAuth,
			newMemoryService,
			jaztools.New,
			terminal.New,
			app.NewACPMCPServerReader,
			exectool.NewCommandManager,
			app.NewPromptBuilder,
			app.NewACPAgentConfigSource,
			app.NewACPConfig,
			acp.NewManager,
			sessionlock.New,
			sessionevents.New,
			threads.NewService,
			app.NewWidgetService,
			app.NewWidgetSessionPublisher,
			app.NewToolRegistry,
			app.NewMCPManager,
			app.NewProviderSource,
			app.NewProvider,
			app.NewVoice,
			app.NewAgent,
		),
		app.UsageModule(),
		fx.Invoke(
			app.ConnectLocalJazAgent,
			app.ConnectACPCompletion,
			app.CloseMemory,
			app.ConfigureMemoryDreamRunner,
			app.StartMemoryScheduler,
			startServer,
			app.StartMCPManager,
		),
	)
	if err := fxApp.Err(); err != nil {
		return conciseError(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := fxApp.Start(ctx); err != nil {
		return conciseError(err)
	}
	<-fxApp.Wait()
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return conciseError(fxApp.Stop(ctx))
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

	Jaz app.Config
}

func loadConfig() (config, error) {
	loaded, err := configloader.Load()
	if err != nil {
		return config{}, err
	}
	return config{Jaz: loaded.Jaz}, nil
}

func parseServeConfig(args serveArgs) (serverconfig.Config, error) {
	fs := flag.NewFlagSet("jaz", flag.ContinueOnError)
	addr := fs.String("addr", ":5299", "HTTP listen address")
	publicURL := fs.String("public-url", "", "URL shown to Jaz clients")
	if err := fs.Parse(args.Args); err != nil {
		return serverconfig.Config{}, err
	}
	return serverconfig.New(*addr, *publicURL), nil
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
	store *sqlitestore.Store,
	manager *acp.Manager,
	locks *sessionlock.Locks,
	events *sessionevents.Bus,
	prompts *coordinator.Builder,
	mcpManager *mcpruntime.Manager,
	workspace app.Workspace,
	stt voice.STT,
	tts voice.TTS,
	modelProviderRuntime provider.Provider,
	providerSource provider.Source,
	logger *log.Logger,
	cfg app.Config,
	catalog acp.AgentCatalog,
	serverConfig serverconfig.Config,
	widgetService *widgets.Service,
	widgetPublisher *widgets.SessionPublisher,
	memory *memoryservice.Service,
	jazTools *jaztools.Service,
	terminals *terminal.Manager,
	threadService *threads.Service,
	deviceAuth *deviceauth.Service,
	routes server.Routes,
) error {
	authKey, err := runtimeauth.Ensure(store.RootDir())
	if err != nil {
		return err
	}
	handler := &server.Server{
		Agent:                a,
		Store:                store,
		Routes:               routes,
		ACP:                  manager,
		MCP:                  mcpManager,
		Locks:                locks,
		Events:               events,
		Threads:              threadService,
		STT:                  stt,
		TTS:                  tts,
		ModelProviderRuntime: reloadableProvider(modelProviderRuntime),
		Providers:            providerSource,
		AgentCatalog:         catalog,
		AuthKey:              authKey,
		Prompts:              prompts,
		Root:                 store.RootDir(),
		Workspace:            string(workspace),
		Log:                  logger.WithPrefix("server"),
		Memory:               memory,
		JazTools:             jazTools,
		Terminal:             terminals,
		Devices:              deviceAuth,
	}
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
		loops.WithPromptExtra(widgetService.LoopPromptExtra),
		loops.WithArtifactSurface(widgetService.LoopArtifactSurface),
	)
	jazTools.SetLoops(loopService)
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
	manager.TurnFinished = func(ctx context.Context, job acp.Job) {
		finishLoopFromACP(loopService, logger, job)
		handler.HandleACPTurnFinished(ctx, job)
	}
	srv := &http.Server{
		Addr:    serverConfig.Addr,
		Handler: handler.Handler(),
	}
	var stopLoops context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fmt.Printf("jaz server listening on %s\n", serverconfig.DisplayAddr(serverConfig.Addr))
			if strings.TrimSpace(serverConfig.PublicURL) == "" {
				fmt.Printf("client: %s\n", serverconfig.ClientURL(serverConfig, authKey))
			} else {
				fmt.Printf("client: %s\n", serverconfig.ClientBaseURL(serverConfig))
				fmt.Printf("client key: %s\n", runtimeauth.Path(store.RootDir()))
			}
			fmt.Printf("root: %s\n", store.RootDir())
			fmt.Printf("workspace: %s\n", workspace)
			if err := loopMemoryPaths.EnsureDir(); err != nil {
				logger.WithPrefix("loops").Warn("loop memory directory unavailable", "path", loopMemoryPaths.Dir(), "error", err)
			}
			if err := loopService.EnsureMemoryPaths(); err != nil {
				return err
			}
			loopCtx, cancelLoops := context.WithCancel(context.Background())
			stopLoops = cancelLoops
			go func() {
				if err := loops.StartScheduler(loopCtx, loopService, 30*time.Second); err != nil && loopCtx.Err() == nil {
					logger.WithPrefix("loops").Error("scheduler stopped", "error", err)
				}
			}()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				handler.PruneManagedWorktrees(ctx)
			}()
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Fprintln(os.Stderr, "serve:", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if stopLoops != nil {
				stopLoops()
			}
			manager.Close()
			return stopHTTPServer(ctx, srv)
		},
	})
	return nil
}

func reloadableProvider(p provider.Provider) provider.ReloadableProvider {
	control, _ := p.(provider.ReloadableProvider)
	return control
}

func finishLoopFromACP(service *loops.Service, logger *log.Logger, job acp.Job) {
	if service == nil || job.ID == "" {
		return
	}
	status := loops.RunStatusOK
	switch job.State {
	case acp.StateFailed:
		status = loops.RunStatusError
	case acp.StateCancelled:
		status = loops.RunStatusCancelled
	}
	if _, ok, err := service.FinishThread(job.ID, status, job.Error); err != nil {
		logger.WithPrefix("loops").Error("finishing loop run from acp state failed", "session", job.ID, "error", err)
	} else if ok {
		logger.WithPrefix("loops").Info("loop run finished", "session", job.ID, "state", job.State)
	}
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
