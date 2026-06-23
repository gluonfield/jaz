package main

import (
	"fmt"
	"io"
)

var (
	version = "dev"
)

func printVersion(w io.Writer) {
	v := version
	if v == "" {
		v = "dev"
	}
	fmt.Fprintf(w, "jaz %s\n", v)
}
