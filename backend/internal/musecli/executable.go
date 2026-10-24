package musecli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wins/jaz/backend/internal/execpath"
)

func Resolve(env map[string]string, binDirs string) (string, error) {
	value := func(key string) string {
		if value := env[key]; value != "" {
			return value
		}
		return os.Getenv(key)
	}
	command := strings.TrimSpace(value("MUSE_CLI"))
	pathList, configured := env["PATH"]
	if !configured {
		pathList = os.Getenv("PATH")
	}
	if binDirs != "" {
		pathList = binDirs + string(os.PathListSeparator) + pathList
	}
	if command == "" {
		command = "muse"
		dir := value("MUSE_INSTALL_DIR")
		if dir == "" {
			dir = filepath.Join(value("HOME"), ".local", "bin")
			if runtime.GOOS == "windows" {
				dir = filepath.Join(value("LOCALAPPDATA"), "Programs", "muse")
			}
		}
		pathList += string(os.PathListSeparator) + dir
	}
	if strings.ContainsAny(command, `/\`) {
		resolved, err := filepath.Abs(command)
		if err != nil {
			return "", err
		}
		command = resolved
	}
	resolved, err := execpath.ResolveInPath(command, pathList)
	if err != nil && runtime.GOOS == "windows" && command == "muse" {
		resolved, err = execpath.ResolveInPath("muse.cmd", pathList)
	}
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}
