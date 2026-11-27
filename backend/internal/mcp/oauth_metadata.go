package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func (c *serverConnection) checkOAuth(ctx context.Context, server mcpconfig.Server) error {
	configured := server.OAuth.StaticClientID() != "" || server.OAuth.Issuer != ""
	if c.oauth == nil {
		if configured && !server.UsesManualAuthorization() && c.localSession == nil {
			return errors.New("OAuth sign-in is unavailable for this MCP server")
		}
		return nil
	}
	if c.oauth.needsAuthorization() {
		return nil
	}
	if source, err := c.oauth.TokenSource(ctx); err != nil || source != nil {
		return err
	}
	if configured {
		return errNeedsAuthorization
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := c.oauth.protectedResourceMetadata(probeCtx, nil, discoveryProactive); err == nil {
		return errNeedsAuthorization
	}
	return nil
}

type authorizationMetadata struct {
	*oauthex.AuthServerMeta
	ResponseIssuerSupported bool
}

func authorizationServerMetadata(ctx context.Context, issuer string, client *http.Client) (*authorizationMetadata, error) {
	transport := &metadataTransport{base: client.Transport}
	if transport.base == nil {
		transport.base = http.DefaultTransport
	}
	metadataClient := *client
	metadataClient.Transport = transport
	metadata, err := auth.GetAuthServerMetadata(ctx, issuer, &metadataClient)
	if err != nil || metadata == nil {
		return nil, err
	}
	return &authorizationMetadata{AuthServerMeta: metadata, ResponseIssuerSupported: transport.issuerSupported}, nil
}

// The SDK validates discovery and metadata, but does not yet expose RFC 9207's flag.
type metadataTransport struct {
	base            http.RoundTripper
	issuerSupported bool
}

func (t *metadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusOK {
		return res, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 1<<20 {
		return nil, errors.New("authorization server metadata exceeds 1 MiB")
	}
	var metadata struct {
		IssuerSupported bool `json:"authorization_response_iss_parameter_supported"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}
	t.issuerSupported = metadata.IssuerSupported
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, nil
}
