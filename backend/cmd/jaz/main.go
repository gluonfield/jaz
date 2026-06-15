package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	args, action := serverArgs(os.Args[1:])
	switch action {
	case mainHelp:
		usage(os.Stdout)
		return
	case mainInvalid:
		usage(os.Stderr)
		os.Exit(2)
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
	mainInvalid
)

func serverArgs(args []string) ([]string, mainAction) {
	if len(args) == 0 {
		return nil, mainRun
	}
	switch args[0] {
	case "serve", "server":
		if len(args) > 1 && isHelp(args[1]) {
			return nil, mainHelp
		}
		return args[1:], mainRun
	case "help":
		return nil, mainHelp
	}
	if isHelp(args[0]) {
		return nil, mainHelp
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
	fmt.Fprintln(w, "usage: jaz [--addr addr] [--public-url url]\n       jaz serve [flags]\n       jaz server [flags]\n\nRun the Jaz backend server.")
}
