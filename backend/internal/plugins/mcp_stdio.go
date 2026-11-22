package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
)

func normalizeStdioMCP(root, data string, raw json.RawMessage, configValues map[string]string) (mcpconfig.Server, error) {
	var config stdioConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return mcpconfig.Server{}, err
	}
	if config.Env == nil {
		config.Env = map[string]string{}
	}
	var err error
	config.Command, err = expandValue(config.Command, root, data, configValues)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if strings.HasPrefix(config.Command, "./") || strings.Contains(config.Command, "..") {
		resolved, err := manifest.Resolve(root, config.Command)
		if err != nil {
			return mcpconfig.Server{}, err
		}
		config.Command = resolved
	}
	if config.Cwd == "" {
		config.Cwd = root
	} else {
		config.Cwd, err = expandValue(config.Cwd, root, data, configValues)
		if err != nil {
			return mcpconfig.Server{}, err
		}
		if !filepath.IsAbs(config.Cwd) {
			config.Cwd = filepath.Join(root, config.Cwd)
		}
		config.Cwd = filepath.Clean(config.Cwd)
		if config.Cwd != data {
			rel, relErr := filepath.Rel(root, config.Cwd)
			if relErr != nil || (rel != "." && !filepath.IsLocal(rel)) {
				return mcpconfig.Server{}, fmt.Errorf("cwd must stay inside plugin root or plugin data")
			}
		}
	}
	if info, err := os.Stat(config.Cwd); config.Cwd != data && (err != nil || !info.IsDir()) {
		return mcpconfig.Server{}, fmt.Errorf("cwd must be an existing directory")
	}
	for i := range config.Args {
		config.Args[i], err = expandValue(config.Args[i], root, data, configValues)
		if err != nil {
			return mcpconfig.Server{}, err
		}
	}
	for name, value := range config.Env {
		switch name {
		case "PLUGIN_ROOT", "PLUGIN_DATA", "CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA":
			return mcpconfig.Server{}, fmt.Errorf("env cannot override reserved variable %s", name)
		}
		config.Env[name], err = expandValue(value, root, data, configValues)
		if err != nil {
			return mcpconfig.Server{}, err
		}
	}
	config.Env["PLUGIN_ROOT"] = root
	config.Env["PLUGIN_DATA"] = data
	config.Env["CLAUDE_PLUGIN_ROOT"] = root
	config.Env["CLAUDE_PLUGIN_DATA"] = data
	input, err := mcpconfig.ValidateInput(mcpconfig.ServerInput{
		Name:      "plugin",
		Transport: mcpconfig.TransportStdio,
		Command:   config.Command,
		Args:      config.Args,
		Env:       config.Env,
		Enabled:   true,
	})
	if err != nil {
		return mcpconfig.Server{}, err
	}
	server := input.Server("", time.Time{}, time.Time{})
	server.Cwd = config.Cwd
	return server, nil
}
