// Package oauthbroker is the client for the hosted HTTPS redirect broker. The
// broker's fixed callback relays an authorization redirect to the Jaz callback
// bound into a state it signed, so authorization servers that accept only a
// registered HTTPS redirect URI can reach any Jaz deployment.
package oauthbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const DefaultURL = "https://jaz.chat/oauth/callback"

// Broker is addressed by its callback URL; an empty URL disables it.
type Broker struct {
	URL string
}

// ClientMetadataURL is the OAuth Client ID Metadata Document the broker host
// publishes for Jaz. Its only redirect URI is the broker callback.
func (b Broker) ClientMetadataURL() string {
	return b.sibling("/oauth/client-metadata.json")
}

// Relay returns the redirect URI and broker-signed state with which the
// authorization response reaches returnURL.
func (b Broker) Relay(ctx context.Context, returnURL string) (redirectURL, state string, err error) {
	body, err := json.Marshal(map[string]string{"return_url": returnURL})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.sibling("/oauth/start"), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("oauth broker unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oauth broker rejected start: %s", resp.Status)
	}
	var out struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode oauth broker response: %w", err)
	}
	if out.State == "" {
		return "", "", errors.New("oauth broker returned no state")
	}
	return b.URL, out.State, nil
}

func (b Broker) sibling(path string) string {
	u, err := url.Parse(b.URL)
	if err != nil || u.Host == "" {
		return b.URL
	}
	u.Path, u.RawQuery, u.Fragment = path, "", ""
	return u.String()
}
