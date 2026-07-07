package acp

import (
	"strings"

	"github.com/wins/jaz/backend/internal/execpath"
)

func ResolveExecutable(executable string) (string, error) {
	return execpath.Resolve(executable)
}

// resolveLoginExecutable prefers managed bundle/tool dirs before PATH.
// Candidates are absolute, so ResolveExecutable does the exec-bit check.
func resolveLoginExecutable(binDir, executable string) (string, error) {
	return execpath.ResolveInDirs(binDir, executable)
}

func executableInPath(executable, pathList string) (string, error) {
	return execpath.ResolveInPath(executable, pathList)
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
