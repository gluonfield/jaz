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

func normalizeStdioMCP(root, data string, raw json.RawMessage, portable bool) (mcpconfig.Server, error) {
	var config stdioConfig
	var err error
	if portable {
		err = decodeStrict(raw, &config)
	} else {
		err = json.Unmarshal(raw, &config)
	}
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return mcpconfig.Server{}, err
	}
	if config.Env == nil {
		config.Env = map[string]string{}
	}
	if portable {
		err = expandPortableStdio(&config, root, data)
	} else {
		err = expandNativeStdio(&config, root, data)
	}
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if info, err := os.Stat(config.Cwd); err != nil || !info.IsDir() {
		return mcpconfig.Server{}, fmt.Errorf("cwd must be an existing directory")
	}
	config.Env["PLUGIN_ROOT"] = root
	config.Env["PLUGIN_DATA"] = data
	input, err := mcpconfig.ValidateInput(mcpconfig.ServerInput{Name: "plugin", Transport: mcpconfig.TransportStdio, Command: config.Command, Args: config.Args, Env: config.Env, Enabled: true})
	if err != nil {
		return mcpconfig.Server{}, err
	}
	server := input.Server("", time.Time{}, time.Time{})
	server.Cwd = config.Cwd
	return server, nil
}

func expandPortableStdio(config *stdioConfig, root, data string) error {
	for _, name := range []string{"PLUGIN_ROOT", "PLUGIN_DATA"} {
		if _, found := config.Env[name]; found {
			return fmt.Errorf("env cannot override %s", name)
		}
	}
	if strings.HasPrefix(config.Command, "./") {
		resolved, err := manifest.Resolve(root, config.Command)
		if err != nil {
			return err
		}
		config.Command = resolved
	} else if strings.ContainsAny(config.Command, "/\\ \t\r\n") {
		return fmt.Errorf("command must be a bare executable or ./ package path")
	}
	base, relative := root, config.Cwd
	switch {
	case relative == "":
		relative = "."
	case relative == "${PLUGIN_ROOT}" || strings.HasPrefix(relative, "${PLUGIN_ROOT}/"):
		relative = "." + strings.TrimPrefix(relative, "${PLUGIN_ROOT}")
	case relative == "${PLUGIN_DATA}" || strings.HasPrefix(relative, "${PLUGIN_DATA}/"):
		base = data
		relative = "." + strings.TrimPrefix(relative, "${PLUGIN_DATA}")
	case !strings.HasPrefix(relative, "./"):
		return fmt.Errorf("cwd must start with ./, ${PLUGIN_ROOT} or ${PLUGIN_DATA}")
	}
	expand := strings.NewReplacer("${PLUGIN_ROOT}", root, "${PLUGIN_DATA}", data).Replace
	var err error
	config.Cwd, err = manifest.Resolve(base, expand(relative))
	if err != nil {
		return err
	}
	for i := range config.Args {
		config.Args[i] = expand(config.Args[i])
	}
	for name, value := range config.Env {
		config.Env[name] = expand(value)
	}
	return nil
}

func expandNativeStdio(config *stdioConfig, root, data string) error {
	var err error
	config.Command, err = expandLegacy(config.Command, root, data)
	if err != nil {
		return err
	}
	if config.Cwd == "" {
		config.Cwd = root
	} else {
		config.Cwd, err = expandLegacy(config.Cwd, root, data)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(config.Cwd) {
			config.Cwd = filepath.Join(root, config.Cwd)
		}
	}
	for i := range config.Args {
		config.Args[i], err = expandLegacy(config.Args[i], root, data)
		if err != nil {
			return err
		}
	}
	for name, value := range config.Env {
		config.Env[name], err = expandLegacy(value, root, data)
		if err != nil {
			return err
		}
	}
	config.Env["CLAUDE_PLUGIN_ROOT"] = root
	config.Env["CLAUDE_PLUGIN_DATA"] = data
	return nil
}
