package acpadapter

import "github.com/wins/jaz/backend/internal/acp"

type adapterSpec struct {
	Adapter  string
	Version  string
	Platform string
	URL      string
	SHA256   string
	Root     string
	Command  string
	Env      map[string]string
	Files    []adapterFile
}

type adapterFile struct {
	URL    string
	SHA256 string
	Source string
	Path   string
}

func (s adapterSpec) launch() acp.AdapterLaunch {
	return acp.AdapterLaunch{Command: s.Command, Env: s.Env}
}

func (s adapterSpec) executables() []string {
	paths := []string{s.Command}
	for _, value := range s.Env {
		paths = append(paths, value)
	}
	for _, file := range s.Files {
		paths = append(paths, file.Path)
	}
	return paths
}
