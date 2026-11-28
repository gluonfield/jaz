package mcp

import (
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type oauthChallenge struct {
	metadataURL string
	scopes      []string
}

func (c *oauthChallenge) hasParameters() bool {
	return c.metadataURL != "" || len(c.scopes) > 0
}

func parseOAuthChallenge(headers []string) (*oauthChallenge, error) {
	challenges, err := oauthex.ParseWWWAuthenticate(headers)
	if err != nil {
		return nil, fmt.Errorf("parse WWW-Authenticate: %w", err)
	}
	var bare *oauthChallenge
	for _, challenge := range challenges {
		if challenge.Scheme != "bearer" {
			continue
		}
		candidate := &oauthChallenge{
			metadataURL: challenge.Params["resource_metadata"],
			scopes:      strings.Fields(challenge.Params["scope"]),
		}
		if candidate.hasParameters() {
			return candidate, nil
		}
		bare = candidate
	}
	return bare, nil
}

func toolOAuthChallenge(result *mcpsdk.CallToolResult) *oauthChallenge {
	if result == nil || !result.IsError {
		return nil
	}
	var headers []string
	switch values := result.Meta["mcp/www_authenticate"].(type) {
	case string:
		headers = []string{values}
	case []any:
		for _, value := range values {
			if header, ok := value.(string); ok {
				headers = append(headers, header)
			}
		}
	}
	challenge, _ := parseOAuthChallenge(headers)
	return challenge
}
