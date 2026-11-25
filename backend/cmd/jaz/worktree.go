package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	configloader "github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/worktrees"
)

func runWorktree(args []string) error {
	if len(args) == 0 || args[0] != "run" && args[0] != "list" && args[0] != "adopt" {
		return fmt.Errorf("usage: jaz worktree run [--workspace path] [--repo path] [--base ref] -- command [arguments]\n       jaz worktree list [--workspace path]\n       jaz worktree adopt [--workspace path] [--repo path] <stopped-checkout>")
	}
	flags := flag.NewFlagSet("worktree "+args[0], flag.ContinueOnError)
	workspace := flags.String("workspace", "", "server workspace")
	var repo, base string
	if args[0] != "list" {
		flags.StringVar(&repo, "repo", ".", "source repository")
	}
	if args[0] == "run" {
		flags.StringVar(&base, "base", "", "branch point, defaults to the source checkout's HEAD")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *workspace == "" || args[0] != "run" {
		cfg, err := configloader.Load()
		if err != nil {
			return err
		}
		root := cfg.Jaz.Root
		if root == "" {
			root = sqlitestore.DefaultRoot()
		}
		if *workspace == "" {
			*workspace = cfg.Jaz.Workspace
			if *workspace == "" {
				*workspace = runtimefiles.New(root).DefaultWorkspace
			}
		}
		if args[0] != "run" {
			sessions, err := sqlitestore.ReadWorktreeSessions(root)
			if err != nil {
				return err
			}
			if args[0] == "adopt" {
				if flags.NArg() != 1 {
					return fmt.Errorf("worktree adopt requires one stopped checkout path")
				}
				return worktrees.Adopt(ctx, *workspace, flags.Arg(0), repo, sessions)
			}
			if flags.NArg() != 0 {
				return fmt.Errorf("worktree list takes no arguments")
			}
			paths, err := worktrees.Review(*workspace, sessions)
			if err != nil {
				return err
			}
			for _, path := range paths {
				fmt.Fprintln(os.Stdout, filepath.Clean(path))
			}
			return nil
		}
	}
	return worktrees.RunJob(ctx, *workspace, repo, base, flags.Args(), os.Stdin, os.Stdout, os.Stderr)
}
