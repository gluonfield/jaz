package acpadapter

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
)

type adapterSpec struct {
	Adapter  string
	Version  string
	Platform string
	URL      string
	SHA256   string
	Root     string
	Command  string
	Env      map[string]string
}

func (s adapterSpec) launch() acp.AdapterLaunch {
	return acp.AdapterLaunch{Command: s.Command, Env: s.Env}
}

func (s adapterSpec) executables() []string {
	paths := []string{s.Command}
	for _, value := range s.Env {
		paths = append(paths, value)
	}
	return paths
}

func (s adapterSpec) binDirs() string {
	var dirs []string
	for _, path := range s.executables() {
		dir := filepath.Dir(path)
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}
