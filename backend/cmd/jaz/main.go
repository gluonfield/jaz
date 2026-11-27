package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/codexauthbridge"
)

func main() {
	if codexauthbridge.Enabled(os.Args[1:]) {
		if err := codexauthbridge.Run(context.Background(), os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "Codex workspace authentication:", err)
			os.Exit(1)
		}
		return
	}
	args, action := commandArgs(os.Args[1:])
	switch action {
	case mainHelp:
		usage(os.Stdout)
		return
	case mainVersion:
		printVersion(os.Stdout)
		return
	case mainInvalid:
		usage(os.Stderr)
		os.Exit(2)
	case mainUpdate:
		if err := runUpdate(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			os.Exit(1)
		}
		return
	case mainConnectCloud:
		if err := runConnectCloud(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "connect-cloud:", err)
			os.Exit(1)
		}
		return
	case mainWorktree:
		if err := runWorktree(args); err != nil {
			fmt.Fprintln(os.Stderr, "worktree:", err)
			os.Exit(1)
		}
		return
	}
	if err := runServe(args); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}

type mainAction int

const (
	mainRun mainAction = iota
	mainHelp
	mainVersion
	mainInvalid
	mainUpdate
	mainConnectCloud
	mainWorktree
)

func commandArgs(args []string) ([]string, mainAction) {
	if len(args) == 0 {
		return nil, mainRun
	}
	switch args[0] {
	case "serve", "server":
		if len(args) > 1 && isHelp(args[1]) {
			return nil, mainHelp
		}
		return args[1:], mainRun
	case "update":
		return args[1:], mainUpdate
	case "connect-cloud":
		return args[1:], mainConnectCloud
	case "worktree":
		return args[1:], mainWorktree
	case "help":
		return nil, mainHelp
	}
	if isHelp(args[0]) {
		return nil, mainHelp
	}
	if args[0] == "--version" || args[0] == "-version" || args[0] == "version" {
		return nil, mainVersion
	}
	if strings.HasPrefix(args[0], "-") {
		return args, mainRun
	}
	return nil, mainInvalid
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: jaz [--addr addr] [--public-url url] [--preview-url-template url]\n       jaz --version\n       jaz update [--latest|--version vX.Y.Z]\n       jaz serve [flags]\n       jaz server [flags]\n       jaz connect-cloud --url <url> --token <token> --device-id <id> [--name <display>] [--grant <id=mode:path>...] [--allow-exec]\n\nRun and administer Jaz.")
	fmt.Fprintln(w, "\nManage Git checkouts with jaz worktree list, run, or adopt.")
}
