package plugins

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"golang.org/x/net/http/httpguts"
)

var variable = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandValue(value, root, data string, configValues map[string]string) (string, error) {
	value = variable.ReplaceAllStringFunc(value, func(token string) string {
		name, fallback, hasDefault := strings.Cut(token[2:len(token)-1], ":-")
		switch name {
		case "PLUGIN_ROOT", "CLAUDE_PLUGIN_ROOT":
			return root
		case "PLUGIN_DATA", "CLAUDE_PLUGIN_DATA":
			return data
		}
		if configValues != nil {
			if configured, ok := configValues[name]; ok && (configured != "" || !hasDefault) {
				return configured
			}
		}
		if envValue, ok := os.LookupEnv(name); ok && (envValue != "" || !hasDefault) {
			return envValue
		}
		if hasDefault {
			return fallback
		}
		return token
	})
	return value, nil
}

func normalizeMCP(root, data string, raw json.RawMessage, configValues map[string]string) (mcpconfig.Server, error) {
	var probe struct {
		Type      string `json:"type"`
		URL       string `json:"url"`
		ServerURL string `json:"serverUrl"`
		Command   string `json:"command"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return mcpconfig.Server{}, err
	}
	if probe.Command != "" && (probe.URL != "" || probe.ServerURL != "") {
		return mcpconfig.Server{}, fmt.Errorf("MCP server cannot specify both command and url")
	}
	if probe.Type == "stdio" || (probe.Command != "" && probe.URL == "" && probe.ServerURL == "") {
		return normalizeStdioMCP(root, data, raw, configValues)
	}
	return normalizeRemoteMCP(root, data, raw, configValues)
}

func normalizeRemoteMCP(root, data string, raw json.RawMessage, configValues map[string]string) (mcpconfig.Server, error) {
	input := mcpconfig.ServerInput{Name: "plugin", Enabled: true}
	var config remoteConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return mcpconfig.Server{}, err
	}
	rawURL := cmp.Or(strings.TrimSpace(config.URL), strings.TrimSpace(config.ServerURL))
	if rawURL == "" {
		return mcpconfig.Server{}, fmt.Errorf("MCP endpoint url is required")
	}
	expandedURL, err := expandValue(rawURL, root, data, configValues)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	for name, value := range config.Headers {
		expanded, err := expandValue(value, root, data, configValues)
		if err != nil {
			return mcpconfig.Server{}, err
		}
		config.Headers[name] = expanded
	}
	switch config.Type {
	case "", "streamable-http", "http":
		input.Transport = mcpconfig.TransportStreamableHTTP
	case "sse":
		input.Transport = mcpconfig.TransportSSE
	default:
		return mcpconfig.Server{}, fmt.Errorf("unknown transport %q", config.Type)
	}
	input.URL = expandedURL
	parsedURL, err := url.Parse(expandedURL)
	if err != nil {
		return mcpconfig.Server{}, fmt.Errorf("invalid MCP endpoint")
	}
	host := parsedURL.Hostname()
	if parsedURL.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return mcpconfig.Server{}, fmt.Errorf("remote MCP endpoints must use HTTPS")
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
