package testexec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func Write(t testing.TB, path, unix, windows string) string {
	t.Helper()
	body := unix
	if runtime.GOOS == "windows" {
		if filepath.Ext(path) == "" {
			path += ".cmd"
		}
		body = windows
		if body == "" {
			body = "@echo off\r\n"
		}
	} else if body == "" {
		body = "#!/bin/sh\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
