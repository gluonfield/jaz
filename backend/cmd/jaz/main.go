package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
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
	case mainDevices:
		if err := runDevices(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "devices:", err)
			os.Exit(1)
		}
		return
	case mainUpdate:
		if err := runUpdate(args, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
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
	mainDevices
	mainUpdate
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
	case "devices":
		return args[1:], mainDevices
	case "update":
		return args[1:], mainUpdate
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
	fmt.Fprintln(w, "usage: jaz [--addr addr] [--public-url url] [--preview-url-template url]\n       jaz --version\n       jaz update [--latest|--version vX.Y.Z]\n       jaz serve [flags]\n       jaz server [flags]\n       jaz devices [--root path]\n       jaz devices [--root path] approve <pairing-or-device-id>\n\nRun and administer Jaz.")
}
