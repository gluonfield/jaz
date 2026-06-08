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
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/app"
	configloader "github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/loops"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"github.com/wins/jaz/backend/internal/voice"
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
			parseServeOptions,
			app.NewStore,
			app.NewWorkspace,
			app.NewMemory,
			exectool.NewCommandManager,
			app.NewPromptBuilder,
			app.NewACPConfig,
			acp.NewManager,
			sessionlock.New,
			sessionevents.New,
			app.NewToolRegistry,
			app.NewMCPManager,
			app.NewProvider,
			app.NewVoice,
			app.NewAgent,
		),
		fx.Invoke(
			app.ConnectACPCompletion,
			app.CloseMemory,
			app.StartMemoryScheduler,
			app.StartMCPManager,
			startServer,
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

type serveOptions struct {
	Addr string
}

func loadConfig() (config, error) {
	loaded, err := configloader.Load()
	if err != nil {
		return config{}, err
	}
	return config{Jaz: loaded.Jaz}, nil
}

func parseServeOptions(args serveArgs) (serveOptions, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	if err := fs.Parse(args.Args); err != nil {
		return serveOptions{}, err
	}
	return serveOptions{Addr: *addr}, nil
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
	logger *log.Logger,
	cfg app.Config,
	opts serveOptions,
) {
	handler := &server.Server{
		Agent:                 a,
		Store:                 store,
		ACP:                   manager,
		MCP:                   mcpManager,
		Locks:                 locks,
		Events:                events,
		STT:                   stt,
		TTS:                   tts,
		NativeModelProvider:   cfg.Provider.Type,
		NativeModel:           cfg.Provider.Model,
		NativeReasoningEffort: cfg.Provider.ReasoningEffort,
		Prompts:               prompts,
		Root:                  store.RootDir(),
		Log:                   logger.WithPrefix("server"),
	}
	loopRunner := server.NewLoopRunner(handler)
	loopService := loops.NewService(store, loopRunner, logger)
	handler.Loops = loopService
	manager.TurnFinished = func(ctx context.Context, job acp.Job) {
		finishLoopFromACP(loopService, logger, job)
		handler.HandleACPTurnFinished(ctx, job)
	}
	srv := &http.Server{
		Addr:    opts.Addr,
		Handler: handler.Handler(),
	}
	var stopLoops context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fmt.Printf("jaz server listening on %s\n", displayAddr(opts.Addr))
			fmt.Printf("root: %s\n", store.RootDir())
			fmt.Printf("workspace: %s\n", workspace)
			loopCtx, cancelLoops := context.WithCancel(context.Background())
			stopLoops = cancelLoops
			go func() {
				if err := loops.StartScheduler(loopCtx, loopService, 30*time.Second); err != nil && loopCtx.Err() == nil {
					logger.WithPrefix("loops").Error("scheduler stopped", "error", err)
				}
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
