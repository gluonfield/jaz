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

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func runServe(args []string) error {
	fxApp := fx.New(
		fx.StopTimeout(15*time.Second),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Supply(serveArgs{Args: args}),
		fx.Provide(
			loadServeConfig,
			app.NewStore,
			app.NewWorkspace,
			exectool.NewCommandManager,
			app.LoadSkills,
			app.NewSkillsPrompt,
			app.NewSystemPrompt,
			app.NewACPConfig,
			acp.NewManager,
			sessionlock.New,
			sessionevents.New,
			app.NewToolRegistry,
			app.NewProvider,
			app.NewAgent,
		),
		fx.Invoke(
			app.ConnectACPCompletion,
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

type serveConfig struct {
	fx.Out

	Config  app.Config
	Options serveOptions
}

type serveOptions struct {
	Addr string
}

func loadServeConfig(args serveArgs) (serveConfig, error) {
	loaded, err := config.Load()
	if err != nil {
		return serveConfig{}, err
	}
	cfg := loaded.Jaz
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	if err := fs.Parse(args.Args); err != nil {
		return serveConfig{}, err
	}
	return serveConfig{Config: cfg, Options: serveOptions{Addr: *addr}}, nil
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
	systemPrompt app.SystemPrompt,
	workspace app.Workspace,
	opts serveOptions,
) {
	srv := &http.Server{
		Addr:    opts.Addr,
		Handler: (&server.Server{Agent: a, Store: store, ACP: manager, Locks: locks, Events: events, SystemPrompt: string(systemPrompt), Root: store.RootDir()}).Handler(),
	}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fmt.Printf("jaz server listening on %s\n", displayAddr(opts.Addr))
			fmt.Printf("root: %s\n", store.RootDir())
			fmt.Printf("workspace: %s\n", workspace)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Fprintln(os.Stderr, "serve:", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			manager.Close()
			return stopHTTPServer(ctx, srv)
		},
	})
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
