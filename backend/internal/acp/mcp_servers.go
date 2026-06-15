package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func (m *Manager) mcpServersForAgent(ctx context.Context, initRaw json.RawMessage) []json.RawMessage {
	var init struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(initRaw, &init); err != nil {
		return []json.RawMessage{}
	}
	if init.AgentCapabilities.MCPCapabilities == nil || !init.AgentCapabilities.MCPCapabilities.HTTP {
		return []json.RawMessage{}
	}
	servers, err := enabledHTTPMCPServers(ctx, m.cfg.MCPStore, m.cfg.MCPTokens)
	if err != nil {
		m.log.Warn("load mcp servers for acp failed", "error", err)
		servers = nil
	}
	if servers == nil {
		return []json.RawMessage{}
	}
	return servers
}

func enabledHTTPMCPServers(ctx context.Context, store mcpconfig.ServerReader, tokens integrationoauth.Store) ([]json.RawMessage, error) {
	if store == nil {
		return nil, nil
	}
	servers, err := store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	var out []json.RawMessage
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		headers, err := resolvedACPHeaders(ctx, server, tokens)
		if err != nil {
			continue
		}
		if headers == nil {
			// codex-acp's schema requires an array; null fails session/new.
			headers = []mcpconfig.Header{}
		}
		payload := struct {
			Type    string             `json:"type"`
			Name    string             `json:"name"`
			URL     string             `json:"url"`
			Headers []mcpconfig.Header `json:"headers"`
		}{
			Type:    "http",
			Name:    server.Name,
			URL:     server.URL,
			Headers: headers,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, nil
}

func resolvedACPHeaders(ctx context.Context, server mcpconfig.Server, tokens integrationoauth.Store) ([]mcpconfig.Header, error) {
	headers, err := mcpconfig.ResolvedHeaders(server, false)
	if err != nil {
		return nil, err
	}
	headers = resolveSessionHeaders(ctx, headers)
	if hasHeader(headers, "Authorization") || tokens == nil || strings.TrimSpace(server.ID) == "" {
		return headers, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token, err := (integrationoauth.Refresher{
		Store: tokens,
	}).FreshToken(ctx, mcpconfig.OAuthConnectionID(server.ID))
	if err != nil {
		if errors.Is(err, integrationoauth.ErrTokenNotFound) {
			return headers, nil
		}
		return nil, err
	}
	tokenType := strings.TrimSpace(token.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return append(headers, mcpconfig.Header{Name: "Authorization", Value: tokenType + " " + strings.TrimSpace(token.AccessToken)}), nil
}

func resolveSessionHeaders(ctx context.Context, headers []mcpconfig.Header) []mcpconfig.Header {
	sessionID := mcpsession.ID(ctx)
	out := make([]mcpconfig.Header, 0, len(headers))
	for _, header := range headers {
		if header.Value != mcpsession.HeaderPlaceholder {
			out = append(out, header)
			continue
		}
		if sessionID != "" {
			header.Value = sessionID
			out = append(out, header)
		}
	}
	return out
}

func hasHeader(headers []mcpconfig.Header, name string) bool {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return true
		}
	}
	return false
}
