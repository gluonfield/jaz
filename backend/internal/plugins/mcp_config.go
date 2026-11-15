package plugins

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"golang.org/x/net/http/httpguts"
)

var variable = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandLegacy(value, root, data string) (string, error) {
	var missing string
	value = variable.ReplaceAllStringFunc(value, func(token string) string {
		name, fallback, hasDefault := strings.Cut(token[2:len(token)-1], ":-")
		switch name {
		case "CLAUDE_PLUGIN_ROOT":
			return root
		case "CLAUDE_PLUGIN_DATA":
			return data
		}
		if value, ok := os.LookupEnv(name); ok && (value != "" || !hasDefault) {
			return value
		}
		if hasDefault {
			return fallback
		}
		missing = name
		return token
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %s is not set", missing)
	}
	return value, nil
}

func normalizeMCP(root, data string, raw json.RawMessage, portable bool) (mcpconfig.Server, error) {
	var kind struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &kind); err != nil {
		return mcpconfig.Server{}, err
	}
	if kind.Type == "stdio" || !portable && kind.Type == "" {
		return normalizeStdioMCP(root, data, raw, portable)
	}
	return normalizeRemoteMCP(root, data, raw, portable)
}

func normalizeRemoteMCP(root, data string, raw json.RawMessage, portable bool) (mcpconfig.Server, error) {
	input := mcpconfig.ServerInput{Name: "plugin", Enabled: true}
	var config remoteConfig
	var err error
	if portable {
		err = decodeStrict(raw, &config)
	} else {
		err = json.Unmarshal(raw, &config)
		if err == nil {
			config.URL, err = expandLegacy(config.URL, root, data)
		}
		for name, value := range config.Headers {
			if err == nil {
				config.Headers[name], err = expandLegacy(value, root, data)
			}
		}
	}
	if err != nil {
		return mcpconfig.Server{}, err
	}
	switch config.Type {
	case "streamable-http", "http":
		if portable && config.Type == "http" {
			return mcpconfig.Server{}, fmt.Errorf("unknown transport %q", config.Type)
		}
		input.Transport = mcpconfig.TransportStreamableHTTP
	case "sse":
		input.Transport = mcpconfig.TransportSSE
	default:
		return mcpconfig.Server{}, fmt.Errorf("unknown transport %q", config.Type)
	}
	input.URL = config.URL
	endpoint, err := url.Parse(config.URL)
	if err != nil {
		return mcpconfig.Server{}, fmt.Errorf("invalid MCP endpoint")
	}
	if portable && strings.EqualFold(endpoint.Scheme, "http") && endpoint.Hostname() != "localhost" && !net.ParseIP(endpoint.Hostname()).IsLoopback() {
		return mcpconfig.Server{}, fmt.Errorf("non-loopback MCP endpoints require HTTPS")
	}
	seen := map[string]bool{}
	for name, value := range config.Headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) || seen[strings.ToLower(name)] {
			return mcpconfig.Server{}, fmt.Errorf("invalid or duplicate header %q", name)
		}
		seen[strings.ToLower(name)] = true
		input.Headers = append(input.Headers, mcpconfig.Header{Name: name, Value: value})
	}
	validated, err := mcpconfig.ValidateInput(input)
	if err != nil {
		return mcpconfig.Server{}, fmt.Errorf("invalid remote MCP configuration")
	}
	return validated.Server("", time.Time{}, time.Time{}), nil
}
