package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
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
	if c.oauth.authorizationChallenge() != nil {
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
	if _, err := c.oauth.protectedResourceMetadata(probeCtx, nil); err == nil {
		return errNeedsAuthorization
	}
	return nil
}

func (h *oauthHandler) protectedResourceMetadata(ctx context.Context, challenge *oauthChallenge) (*oauthex.ProtectedResourceMetadata, error) {
	resource, err := url.Parse(h.serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	origin := url.URL{Scheme: resource.Scheme, Host: resource.Host}
	for _, candidate := range protectedResourceMetadataURLs(resource, origin, challenge) {
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate.url, candidate.resource, h.httpClient)
		if err != nil {
			continue
		}
		if len(prm.AuthorizationServers) == 0 {
			return nil, errors.New("protected resource metadata lists no authorization servers")
		}
		return prm, nil
	}
	issuer := h.oauth.Issuer
	if issuer == "" {
		if challenge == nil {
			return nil, errors.New("server did not advertise OAuth protected resource metadata")
		}
		issuer = origin.String()
	}
	return &oauthex.ProtectedResourceMetadata{
		AuthorizationServers: []string{issuer},
		Resource:             h.serverURL,
	}, nil
}

type prmCandidate struct {
	url      string
	resource string
}

func protectedResourceMetadataURLs(resource *url.URL, origin url.URL, challenge *oauthChallenge) []prmCandidate {
	const wellKnown = "/.well-known/oauth-protected-resource"
	var candidates []prmCandidate
	identifier := resource.String()
	if challenge != nil && challenge.metadataURL != "" {
		candidates = append(candidates, prmCandidate{url: challenge.metadataURL, resource: identifier})
	}
	metadata := origin
	for _, path := range []string{wellKnown + strings.TrimRight(resource.Path, "/"), wellKnown} {
		metadata.Path = path
		candidate := prmCandidate{url: metadata.String(), resource: identifier}
		if !slices.Contains(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	if root := origin.String(); root != identifier {
		candidates = append(candidates, prmCandidate{url: metadata.String(), resource: root})
	}
	return candidates
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
