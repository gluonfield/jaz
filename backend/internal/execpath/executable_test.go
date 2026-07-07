package execpath

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveFindsExecutableOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable-bit test")
	}
	bin := t.TempDir()
	exe := writeExecutable(t, bin, "jaz-execpath-test")
	t.Setenv("PATH", bin)
	t.Setenv("SHELL", "/bin/sh")

	got, err := Resolve(filepath.Base(exe))
	if err != nil {
		t.Fatal(err)
	}
	if got != exe {
		t.Fatalf("resolved path = %q, want %q", got, exe)
	}
}

func TestResolveInDirsPrefersExplicitDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable-bit test")
	}
	preferredBin := t.TempDir()
	pathBin := t.TempDir()
	exeName := "jaz-execpath-test"
	preferred := writeExecutable(t, preferredBin, exeName)
	writeExecutable(t, pathBin, exeName)
	t.Setenv("PATH", pathBin)
	t.Setenv("SHELL", "/bin/sh")

	got, err := ResolveInDirs(preferredBin, exeName)
	if err != nil {
		t.Fatal(err)
	}
	if got != preferred {
		t.Fatalf("resolved path = %q, want %q", got, preferred)
	}
}

func TestResolveInPathIgnoresAmbientPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable-bit test")
	}
	pathBin := t.TempDir()
	exeName := "jaz-execpath-test"
	writeExecutable(t, pathBin, exeName)
	t.Setenv("PATH", pathBin)
	t.Setenv("SHELL", "/bin/sh")

	_, err := ResolveInPath(exeName, t.TempDir())
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("error = %v, want exec.ErrNotFound", err)
	}
}

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
