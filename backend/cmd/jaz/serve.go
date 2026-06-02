package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/server"
	"go.uber.org/fx"
)

func runServe(args []string) error {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.Supply(serveArgs{Args: args}),
		fx.Provide(
			loadServeConfig,
			app.BuildRuntime,
		),
		fx.Invoke(
			startServer,
		),
	).Run()
	return nil
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
	fs.StringVar(&cfg.Root, "root", cfg.Root, "Jaz root directory")
	fs.StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "default workspace")
	fs.StringVar(&cfg.Provider.Type, "provider", cfg.Provider.Type, "provider: openai, openrouter, or mock")
	fs.StringVar(&cfg.Provider.APIKey, "api-key", cfg.Provider.APIKey, "provider API key")
	fs.StringVar(&cfg.Provider.Model, "model", cfg.Provider.Model, "model name")
	if err := fs.Parse(args.Args); err != nil {
		return serveConfig{}, err
	}
	return serveConfig{Config: cfg, Options: serveOptions{Addr: *addr}}, nil
}

func startServer(lc fx.Lifecycle, runtime *app.Runtime, cfg app.Config, opts serveOptions) {
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = runtime.Store.DefaultWorkspace()
	}
	srv := &http.Server{
		Addr:    opts.Addr,
		Handler: (&server.Server{Agent: runtime.Agent, Store: runtime.Store, ACP: runtime.ACP, Locks: runtime.Locks, Events: runtime.Events, SystemPrompt: runtime.SystemPrompt}).Handler(),
	}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fmt.Printf("jaz server listening on %s\n", displayAddr(opts.Addr))
			fmt.Printf("root: %s\n", runtime.Store.RootDir())
			fmt.Printf("workspace: %s\n", workspace)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Fprintln(os.Stderr, "serve:", err)
				}
			}()
			return nil
		},
		OnStop: srv.Shutdown,
	})
}
