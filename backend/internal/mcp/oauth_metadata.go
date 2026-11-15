package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

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
