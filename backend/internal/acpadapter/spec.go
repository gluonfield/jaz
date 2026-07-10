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
