package execpath

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Resolve(executable string) (string, error) {
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

func ResolveInDirs(dirs, executable string) (string, error) {
	for _, dir := range filepath.SplitList(strings.TrimSpace(dirs)) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		for _, name := range []string{executable, executable + ".exe"} {
			if resolved, err := Resolve(filepath.Join(dir, name)); err == nil {
				return resolved, nil
			}
		}
	}
	return Resolve(executable)
}

func ResolveInPath(executable, pathList string) (string, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" || filepath.IsAbs(executable) {
		return Resolve(executable)
	}
	for _, dir := range filepath.SplitList(pathList) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		for _, name := range []string{executable, executable + ".exe"} {
			if resolved, err := Resolve(filepath.Join(dir, name)); err == nil {
				return resolved, nil
			}
		}
	}
	return "", exec.ErrNotFound
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
