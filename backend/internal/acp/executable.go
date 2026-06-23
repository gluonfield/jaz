package acp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ResolveExecutable(executable string) (string, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return "", fmt.Errorf("command is not configured")
	}
	if filepath.IsAbs(executable) {
		info, err := os.Stat(executable)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", executable)
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", executable)
		}
		return executable, nil
	}
	if path, err := exec.LookPath(executable); err == nil {
		return path, nil
	}
	return loginShellExecutable(executable)
}

// resolveLoginExecutable finds an agent's login CLI, preferring the managed
// adapter's bundle directory so a Node-free backend signs in with the bundled
// binary (e.g. claude ships its CLI in the adapter bundle) instead of requiring
// it on PATH. Falls back to PATH resolution when the bundle has no such binary
// (e.g. the codex adapter bundles codex-acp but not the codex login CLI). The
// bundle candidates are absolute, so ResolveExecutable does the stat/exec-bit
// check for us.
func resolveLoginExecutable(binDir, executable string) (string, error) {
	if dir := strings.TrimSpace(binDir); dir != "" {
		for _, name := range []string{executable, executable + ".exe"} {
			if resolved, err := ResolveExecutable(filepath.Join(dir, name)); err == nil {
				return resolved, nil
			}
		}
	}
	return ResolveExecutable(executable)
}

func loginShellExecutable(executable string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", exec.ErrNotFound
	}
	if strings.ContainsAny(executable, `/\`+"\x00") {
		return "", exec.ErrNotFound
	}
	shell := os.Getenv("SHELL")
	if strings.TrimSpace(shell) == "" {
		shell = "/bin/zsh"
	}
	out, err := exec.Command(shell, "-lc", "command -v "+shellQuote(executable)).Output()
	if err != nil && shell != "/bin/sh" {
		out, err = exec.Command("/bin/sh", "-lc", "command -v "+shellQuote(executable)).Output()
	}
	path := strings.TrimSpace(string(out))
	if err != nil || path == "" {
		return "", exec.ErrNotFound
	}
	return strings.Split(path, "\n")[0], nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\"'\\$`!|&;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
