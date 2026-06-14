package main

import (
	"fmt"
	"os"

	"github.com/wins/jaz/backend/internal/chatcmd"
)

func main() {
	if err := chatcmd.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "jaz-chat:", err)
		os.Exit(1)
	}
}
