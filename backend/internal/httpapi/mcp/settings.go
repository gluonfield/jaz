package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

// mcpServerInput is the settings a client sends, where a missing enabled
// keeps the server's current state.
type mcpServerInput struct {
	mcpconfig.ServerInput
	Enabled     *bool              `json:"enabled,omitempty"`
	BearerToken *string            `json:"bearer_token,omitempty"`
	Headers     *[]mcpHeaderInput  `json:"headers,omitempty"`
	Env         map[string]*string `json:"env,omitempty"`
}

type mcpHeaderInput struct {
	Name   string  `json:"name"`
	Value  *string `json:"value,omitempty"`
	EnvVar string  `json:"envvar,omitempty"`
}

type mcpHeaderView struct {
	Name     string `json:"name"`
	EnvVar   string `json:"envvar,omitempty"`
	ValueSet bool   `json:"value_set,omitempty"`
}

type ServerView struct {
	mcpconfig.Server
	Status         string                 `json:"status"`
	ToolCount      int                    `json:"tool_count"`
	Tools          []mcpconfig.ServerTool `json:"tools,omitempty"`
	Error          string                 `json:"error,omitempty"`
	BearerTokenSet bool                   `json:"bearer_token_set,omitempty"`
	Headers        []mcpHeaderView        `json:"headers,omitempty"`
	Env            map[string]*string     `json:"env,omitempty"`
}

func DecodeServerInput(r *http.Request, current *mcpconfig.Server) (mcpconfig.ServerInput, error) {
	var req mcpServerInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return mcpconfig.ServerInput{}, err
	}
	input := req.ServerInput
	input.Enabled = current == nil || current.Enabled
	if req.Enabled != nil {
		input.Enabled = *req.Enabled
	}
	sameEndpoint := current != nil && current.Transport != mcpconfig.TransportStdio && input.Transport != mcpconfig.TransportStdio && input.Command == "" && strings.TrimSpace(input.URL) == current.URL
	if sameEndpoint {
		input.BearerToken = current.BearerToken
		input.Headers = current.Headers
	}
	if req.BearerToken != nil {
		input.BearerToken = *req.BearerToken
	}
	if req.Headers != nil {
		input.Headers = make([]mcpconfig.Header, 0, len(*req.Headers))
		for _, header := range *req.Headers {
			value := ""
			if header.Value != nil {
				value = *header.Value
			} else if sameEndpoint && strings.TrimSpace(header.EnvVar) == "" {
				for _, saved := range current.Headers {
					if strings.EqualFold(saved.Name, strings.TrimSpace(header.Name)) {
						value = saved.Value
						break
					}
				}
			}
			input.Headers = append(input.Headers, mcpconfig.Header{Name: header.Name, Value: value, EnvVar: header.EnvVar})
		}
	}
	if req.Env != nil {
		input.Env = make(map[string]string, len(req.Env))
		for name, value := range req.Env {
			if value != nil {
				input.Env[name] = *value
				continue
			}
			if current == nil || current.Transport != mcpconfig.TransportStdio || strings.TrimSpace(input.Command) != current.Command || !slices.Equal(input.Args, current.Args) {
				return input, fmt.Errorf("environment variable %s needs a value for the new command", name)
			}
			saved, ok := current.Env[name]
			if !ok {
				return input, fmt.Errorf("environment variable %s has no saved value", name)
			}
			input.Env[name] = saved
		}
	}
	return mcpconfig.ValidateInput(input)
}

func NewServerView(server mcpconfig.Server, status mcpconfig.ServerStatus) ServerView {
	view := ServerView{Server: server, Status: status.Status, ToolCount: status.ToolCount, Tools: status.Tools, Error: status.Error, BearerTokenSet: server.BearerToken != ""}
	view.Server.BearerToken = ""
	view.Server.Headers = nil
	view.Server.Env = nil
	if len(server.Env) != 0 {
		view.Env = make(map[string]*string, len(server.Env))
		for name := range server.Env {
			view.Env[name] = nil
		}
	}
	for _, header := range server.Headers {
		view.Headers = append(view.Headers, mcpHeaderView{Name: header.Name, EnvVar: header.EnvVar, ValueSet: header.Value != ""})
	}
	return view
}
