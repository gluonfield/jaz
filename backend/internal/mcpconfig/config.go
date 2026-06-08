package mcpconfig

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const TransportStreamableHTTP = "streamable_http"

var (
	headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+\-/=?^_` + "`" + `{|}~.]+$`)
	envVarPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EnvHeader struct {
	Name   string `json:"name"`
	EnvVar string `json:"env_var"`
}

type Server struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Transport         string      `json:"transport"`
	URL               string      `json:"url"`
	Enabled           bool        `json:"enabled"`
	BearerTokenEnvVar string      `json:"bearer_token_env_var,omitempty"`
	Headers           []Header    `json:"headers,omitempty"`
	EnvHeaders        []EnvHeader `json:"env_headers,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type ServerInput struct {
	Name              string      `json:"name"`
	URL               string      `json:"url"`
	Enabled           bool        `json:"enabled"`
	BearerTokenEnvVar string      `json:"bearer_token_env_var,omitempty"`
	Headers           []Header    `json:"headers,omitempty"`
	EnvHeaders        []EnvHeader `json:"env_headers,omitempty"`
}

type ServerStatus struct {
	Status    string    `json:"status"`
	ToolCount int       `json:"tool_count"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
}

type ServerReader interface {
	ListMCPServers() ([]Server, error)
}

type Store interface {
	ServerReader
	LoadMCPServer(id string) (Server, error)
	CreateMCPServer(input ServerInput) (Server, error)
	UpdateMCPServer(id string, input ServerInput) (Server, error)
	DeleteMCPServer(id string) error
	SetMCPServerEnabled(id string, enabled bool) (Server, error)
}

func ValidateInput(input ServerInput) (ServerInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	input.BearerTokenEnvVar = strings.TrimSpace(input.BearerTokenEnvVar)
	if input.Name == "" {
		return input, errors.New("name is required")
	}
	if input.URL == "" {
		return input, errors.New("url is required")
	}
	parsed, err := url.Parse(input.URL)
	if err != nil {
		return input, fmt.Errorf("invalid url: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return input, errors.New("url must use http or https")
	}
	if parsed.Host == "" {
		return input, errors.New("url host is required")
	}
	if parsed.User != nil {
		return input, errors.New("url must not include credentials")
	}
	if parsed.Fragment != "" {
		return input, errors.New("url must not include a fragment")
	}
	input.URL = parsed.String()
	if input.BearerTokenEnvVar != "" && !envVarPattern.MatchString(input.BearerTokenEnvVar) {
		return input, fmt.Errorf("invalid bearer token env var: %s", input.BearerTokenEnvVar)
	}
	headers, err := normalizeHeaders(input.Headers)
	if err != nil {
		return input, err
	}
	envHeaders, err := normalizeEnvHeaders(input.EnvHeaders)
	if err != nil {
		return input, err
	}
	input.Headers = headers
	input.EnvHeaders = envHeaders
	return input, nil
}

func normalizeHeaders(headers []Header) ([]Header, error) {
	var out []Header
	for _, header := range headers {
		header.Name = strings.TrimSpace(header.Name)
		if header.Name == "" && header.Value == "" {
			continue
		}
		if header.Name == "" {
			return nil, errors.New("header name is required")
		}
		if !headerNamePattern.MatchString(header.Name) {
			return nil, fmt.Errorf("invalid header name: %s", header.Name)
		}
		out = append(out, header)
	}
	return out, nil
}

func normalizeEnvHeaders(headers []EnvHeader) ([]EnvHeader, error) {
	var out []EnvHeader
	for _, header := range headers {
		header.Name = strings.TrimSpace(header.Name)
		header.EnvVar = strings.TrimSpace(header.EnvVar)
		if header.Name == "" && header.EnvVar == "" {
			continue
		}
		if header.Name == "" {
			return nil, errors.New("env header name is required")
		}
		if header.EnvVar == "" {
			return nil, fmt.Errorf("env var is required for header %s", header.Name)
		}
		if !headerNamePattern.MatchString(header.Name) {
			return nil, fmt.Errorf("invalid header name: %s", header.Name)
		}
		if !envVarPattern.MatchString(header.EnvVar) {
			return nil, fmt.Errorf("invalid env var for header %s: %s", header.Name, header.EnvVar)
		}
		out = append(out, header)
	}
	return out, nil
}

func ResolvedHeaders(server Server, requireEnv bool) ([]Header, error) {
	seen := map[string]int{}
	var out []Header
	add := func(name, value string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		header := Header{Name: name, Value: value}
		key := strings.ToLower(name)
		if idx, ok := seen[key]; ok {
			out[idx] = header
			return
		}
		seen[key] = len(out)
		out = append(out, header)
	}
	for _, header := range server.Headers {
		if strings.TrimSpace(header.Name) != "" {
			add(header.Name, header.Value)
		}
	}
	for _, header := range server.EnvHeaders {
		if strings.TrimSpace(header.Name) == "" || strings.TrimSpace(header.EnvVar) == "" {
			continue
		}
		value := os.Getenv(strings.TrimSpace(header.EnvVar))
		if value == "" {
			if requireEnv {
				return nil, fmt.Errorf("environment variable %s is not set", header.EnvVar)
			}
			continue
		}
		add(header.Name, value)
	}
	if envVar := strings.TrimSpace(server.BearerTokenEnvVar); envVar != "" {
		value := os.Getenv(envVar)
		if value == "" {
			if requireEnv {
				return nil, fmt.Errorf("environment variable %s is not set", envVar)
			}
		} else {
			add("Authorization", "Bearer "+value)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
