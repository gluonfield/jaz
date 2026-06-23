package main

import (
	"fmt"
	"io"
)

var version = "dev"

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "jaz %s\n", version)
}
