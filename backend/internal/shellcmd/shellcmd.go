package shellcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DefaultShell() string {
	if runtime.GOOS == "windows" {
		if shell := strings.TrimSpace(os.Getenv("ComSpec")); shell != "" {
			return shell
		}
		return "cmd.exe"
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func Command(shell, command string, login bool) (string, []string) {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		shell = DefaultShell()
	}
	return commandFor(runtime.GOOS, shell, command, login)
}

func commandFor(goos, shell, command string, login bool) (string, []string) {
	base := strings.ToLower(filepath.Base(shell))
	switch {
	case isPOSIXShell(base):
		flag := "-c"
		if login {
			flag = "-lc"
		}
		return shell, []string{flag, command}
	case base == "powershell.exe" || base == "powershell" || base == "pwsh.exe" || base == "pwsh":
		return shell, []string{"-NoLogo", "-NoProfile", "-Command", command}
	case goos == "windows":
		return shell, []string{"/d", "/s", "/c", command}
	default:
		flag := "-c"
		if login {
			flag = "-lc"
		}
		return shell, []string{flag, command}
	}
}

func isPOSIXShell(base string) bool {
	switch base {
	case "sh", "sh.exe", "bash", "bash.exe", "zsh", "zsh.exe", "fish", "fish.exe":
		return true
	default:
		return false
	}
}
