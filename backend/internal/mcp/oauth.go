package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

const signInRequired = "Sign in required"

var errNeedsAuthorization = errors.New("MCP sign-in required. Open Settings → MCP to sign in")

func isRejectedGrant(err error) bool {
	var rejected *oauth2.RetrieveError
	return errors.As(err, &rejected) && rejected.ErrorCode == "invalid_grant"
}

// oauthHandler implements auth.OAuthHandler for a single MCP server. It is backed
// by the persistent token store: TokenSource serves stored (refreshing) tokens, and
// Authorize — only when interactive — runs the browser authorization-code flow and
// persists the result.
type oauthHandler struct {
	tokenID    string
	serverURL  string
	oauth      mcpconfig.OAuthConfig
	store      integrationoauth.Store
	httpClient *http.Client
	guard      *integrationoauth.RefreshGuard

	redirectURL string
	broker      oauthbroker.Broker
	fetch       codeFetcher

	mu         sync.Mutex
	mode       oauthMode
	state      authorizationState
	challenges []oauthex.Challenge
}

// codeFetcher directs the user through the authorization URL (e.g. opens a browser)
// and returns the authorization code and state once the redirect arrives.
type codeFetcher func(ctx context.Context, authURL string) (mcpconfig.AuthorizationResponse, error)

type metadataDiscovery int

const (
	discoveryFromChallenge metadataDiscovery = iota
	discoveryProactive
)

type oauthClientConfig struct {
	clientID     string
	clientSecret string
	authStyle    oauth2.AuthStyle
	static       bool
	useBroker    bool
}

type oauthMode int

const (
	oauthModeBackground oauthMode = iota
	oauthModeInteractive
)

type authorizationState int

const (
	authorizationIdle authorizationState = iota
	authorizationRequired
	authorizationComplete
)

func newOAuthHandler(server mcpconfig.Server, store integrationoauth.Store, httpClient *http.Client, guard *integrationoauth.RefreshGuard) *oauthHandler {
	return &oauthHandler{
		tokenID:    server.TokenID(),
		serverURL:  server.URL,
		oauth:      server.OAuth,
		store:      store,
		httpClient: httpClient,
		guard:      guard,
		mode:       oauthModeBackground,
		state:      authorizationIdle,
	}
}

// TokenSource is rebuilt per call: the SDK passes the current connection's
// context, and a source cached across a redial would refresh on a closed one.
func (h *oauthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if h.mode == oauthModeInteractive && !h.didAuthorize() {
		return nil, nil
	}
	src, err := (integrationoauth.Refresher{
		Store:        h.store,
		HTTPClient:   h.httpClient,
		ClientConfig: h.refreshClientConfig,
		Guard:        h.guard,
	}).TokenSource(ctx, h.tokenID)
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return nil, nil
	}
	return src, err
}

func (h *oauthHandler) Authorize(ctx context.Context, _ *http.Request, resp *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if h.didAuthorize() {
		return errors.New("MCP server rejected the new OAuth grant")
	}
	headers := resp.Header.Values("WWW-Authenticate")
	challenges, err := bearerChallenges(headers)
	if err != nil {
		return err
	}
	if len(challenges) == 0 && (len(headers) != 0 || resp.StatusCode != http.StatusUnauthorized) {
		return fmt.Errorf("MCP server returned HTTP %d without an OAuth challenge", resp.StatusCode)
	}
	return h.authorizeChallenge(ctx, challenges)
}

func (h *oauthHandler) AuthorizeFromMetadata(ctx context.Context) error {
	return h.authorize(ctx, nil, discoveryProactive)
}

func (h *oauthHandler) authorizeChallenge(ctx context.Context, challenges []oauthex.Challenge) error {
	return h.authorize(ctx, challenges, discoveryFromChallenge)
}

func (h *oauthHandler) authorize(ctx context.Context, challenges []oauthex.Challenge, discovery metadataDiscovery) error {
	if h.mode != oauthModeInteractive {
		h.markAuthorizationRequired(challenges)
		return errNeedsAuthorization
	}
	tok, err := h.runAuthorization(ctx, challenges, discovery)
	if err != nil {
		return err
	}
	return h.saveToken(ctx, tok)
}

func (h *oauthHandler) saveToken(ctx context.Context, tok integrationoauth.Token) error {
	unlock := integrationoauth.LockToken(h.tokenID)
	defer unlock()
	if err := h.store.SaveToken(ctx, h.tokenID, tok); err != nil {
		return fmt.Errorf("persist token: %w", err)
	}
	if h.guard != nil {
		h.guard.Reset(h.tokenID)
	}
	h.mu.Lock()
	h.state = authorizationComplete
	h.mu.Unlock()
	return nil
}

func (h *oauthHandler) needsAuthorization() bool {
	_, required := h.authorizationChallenge()
	return required
}

func (h *oauthHandler) authorizationChallenge() ([]oauthex.Challenge, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.challenges, h.state == authorizationRequired
}

func (h *oauthHandler) didAuthorize() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state == authorizationComplete
}

func (h *oauthHandler) markAuthorizationRequired(challenges []oauthex.Challenge) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state = authorizationRequired
	if len(challenges) > 0 {
		h.challenges = challenges
	}
}

// runAuthorization performs the OAuth 2.0 authorization-code flow with PKCE.
// It uses configured client credentials when present and otherwise follows MCP
// client registration: a client ID metadata document, then dynamic registration.
func (h *oauthHandler) runAuthorization(ctx context.Context, challenges []oauthex.Challenge, discovery metadataDiscovery) (integrationoauth.Token, error) {
	var zero integrationoauth.Token
	prm, err := h.protectedResourceMetadata(ctx, challenges, discovery)
	if err != nil {
		return zero, err
	}
	if len(prm.AuthorizationServers) == 0 {
		return zero, errors.New("server advertised no authorization servers")
	}

	asm, err := authorizationServerMetadata(ctx, prm.AuthorizationServers[0], h.httpClient)
	if err != nil {
		return zero, fmt.Errorf("authorization server metadata: %w", err)
	}
	if asm == nil || !slices.Contains(asm.CodeChallengeMethodsSupported, "S256") {
		return zero, errors.New("authorization server must advertise S256 PKCE support in its OAuth metadata")
	}

	scopes := scopesFromChallenges(challenges)
	if len(scopes) == 0 && !h.oauth.UseAppScopes {
		scopes = prm.ScopesSupported
	}
	clientConfig, err := h.authorizationClientConfig(ctx, asm.AuthServerMeta)
	if err != nil {
		return zero, err
	}
	redirectURL, state, err := h.authorizationRedirect(ctx, clientConfig)
	if err != nil {
		return zero, err
	}

	cfg := &oauth2.Config{
		ClientID:     clientConfig.clientID,
		ClientSecret: clientConfig.clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   asm.AuthorizationEndpoint,
			TokenURL:  asm.TokenEndpoint,
			AuthStyle: clientConfig.authStyle,
		},
		RedirectURL: redirectURL,
		Scopes:      scopes,
	}

	verifier := oauth2.GenerateVerifier()
	authURL := cfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("resource", prm.Resource),
	)

	response, err := h.fetch(ctx, authURL)
	if err != nil {
		return zero, err
	}
	if response.State != state {
		return zero, errors.New("authorization state mismatch")
	}
	if response.Issuer != "" && response.Issuer != asm.Issuer || asm.ResponseIssuerSupported && response.Issuer == "" {
		return zero, errors.New("authorization response issuer is missing or does not match the authorization server")
	}
	if response.Error != "" {
		return zero, fmt.Errorf("authorization failed: %s", response.Error)
	}
	if response.Code == "" {
		return zero, errors.New("authorization returned no code")
	}

	token, err := cfg.Exchange(h.exchangeContext(ctx), response.Code,
		oauth2.VerifierOption(verifier),
		oauth2.SetAuthURLParam("resource", prm.Resource),
	)
	if err != nil {
		return zero, fmt.Errorf("token exchange: %w", err)
	}

	return integrationoauth.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     clientConfig.clientID,
		Issuer:       asm.Issuer,
		ClientSecret: dynamicClientSecret(clientConfig),
		AuthURL:      asm.AuthorizationEndpoint,
		TokenURL:     asm.TokenEndpoint,
		AuthStyle:    int(clientConfig.authStyle),
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Resource:     prm.Resource,
	}, nil
}

func (h *oauthHandler) authorizationClientConfig(ctx context.Context, asm *oauthex.AuthServerMeta) (oauthClientConfig, error) {
	if id := h.oauth.StaticClientID(); id != "" {
		if h.oauth.Issuer != "" && h.oauth.Issuer != asm.Issuer {
			return oauthClientConfig{}, errors.New("configured OAuth client issuer does not match the server's authorization server; update the OAuth client settings")
		}
		saved, ok, err := h.store.LoadToken(ctx, h.tokenID)
		if err != nil {
			return oauthClientConfig{}, err
		}
		if ok && saved.Issuer != "" && saved.Issuer != asm.Issuer && saved.ClientID == id {
			return oauthClientConfig{}, errors.New("authorization server changed; register a client with the new issuer and update the OAuth client settings")
		}
		return h.staticClientConfig(id)
	}
	if asm.ClientIDMetadataDocumentSupported && h.broker.URL != "" {
		return oauthClientConfig{clientID: h.broker.ClientMetadataURL(), authStyle: oauth2.AuthStyleInParams, useBroker: true}, nil
	}
	if asm.RegistrationEndpoint == "" {
		return oauthClientConfig{}, h.clientSetupError("authorization server does not support dynamic client registration")
	}
	applicationType := "web"
	callback, err := url.Parse(h.redirectURL)
	if err != nil {
		return oauthClientConfig{}, err
	}
	if callback.Hostname() == "localhost" || callback.Hostname() == "127.0.0.1" || callback.Hostname() == "::1" {
		applicationType = "native"
	}
	reg, err := oauthex.RegisterClient(ctx, asm.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
		ApplicationType:         applicationType,
		RedirectURIs:            []string{h.redirectURL},
		ClientName:              "Jaz",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	}, h.httpClient)
	if err != nil {
		return oauthClientConfig{}, h.clientSetupError(fmt.Sprintf("automatic client registration failed (%v); the provider may require authenticated registration", err))
	}
	authStyle := oauth2.AuthStyleInParams
	switch reg.TokenEndpointAuthMethod {
	case "client_secret_basic":
		authStyle = oauth2.AuthStyleInHeader
	case "none", "client_secret_post":
	case "":
		if reg.ClientSecret != "" {
			authStyle = oauth2.AuthStyleAutoDetect
		}
	default:
		return oauthClientConfig{}, h.clientSetupError("registered client requires an unsupported token endpoint authentication method")
	}
	return oauthClientConfig{clientID: reg.ClientID, clientSecret: reg.ClientSecret, authStyle: authStyle}, nil
}

func (h *oauthHandler) authorizationRedirect(ctx context.Context, client oauthClientConfig) (string, string, error) {
	if client.useBroker {
		return h.broker.Relay(ctx, h.redirectURL)
	}
	state, err := randomState()
	return h.redirectURL, state, err
}

func (h *oauthHandler) clientSetupError(reason string) error {
	return fmt.Errorf("%s; register an OAuth client with callback %s and S256 PKCE, then configure an OAuth client ID in Settings → MCP → Edit → Advanced → OAuth (set a client secret environment variable only if required)", reason, h.broker.RedirectURL(h.redirectURL))
}

func (h *oauthHandler) refreshClientConfig(_ context.Context, token integrationoauth.Token) (integrationoauth.ClientConfig, error) {
	config := integrationoauth.ClientConfig{
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AuthURL:      token.AuthURL,
		TokenURL:     token.TokenURL,
		AuthStyle:    oauth2.AuthStyle(token.AuthStyle),
		RedirectURL:  token.RedirectURL,
		Scopes:       token.Scopes,
	}
	if token.ClientID != "" && token.ClientID == h.oauth.PublicClientID && token.ClientID != h.oauth.ClientID {
		return config, nil
	}
	if id := h.oauth.StaticClientID(); id != "" {
		if token.ClientID != "" && token.ClientID != id {
			return integrationoauth.ClientConfig{}, errors.New("OAuth client changed; sign in again")
		}
		client, err := h.staticClientConfig(id)
		if err != nil {
			return integrationoauth.ClientConfig{}, err
		}
		config.ClientID = client.clientID
		config.ClientSecret = client.clientSecret
		config.AuthStyle = client.authStyle
	}
	return config, nil
}

func (h *oauthHandler) staticClientConfig(id string) (oauthClientConfig, error) {
	client := oauthClientConfig{clientID: id, authStyle: oauth2.AuthStyleInParams, static: true}
	if h.oauth.UsesLoopback() {
		client.useBroker = h.oauth.PublicClientUseBroker
		return client, nil
	}
	client.useBroker = h.broker.URL != ""
	if env := h.oauth.ClientSecretEnvVar; env != "" {
		client.clientSecret = os.Getenv(env)
		if client.clientSecret == "" {
			return oauthClientConfig{}, fmt.Errorf("environment variable %s is not set", env)
		}
		client.authStyle = oauth2.AuthStyleAutoDetect
	}
	return client, nil
}

func dynamicClientSecret(client oauthClientConfig) string {
	if client.static {
		return ""
	}
	return client.clientSecret
}

func (h *oauthHandler) exchangeContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, h.httpClient)
}

// protectedResourceMetadata discovers the protected resource metadata, trying the
// URL from the WWW-Authenticate challenge, then the well-known locations, and
// finally falling back to treating the server origin as the authorization server.
func (h *oauthHandler) protectedResourceMetadata(ctx context.Context, challenges []oauthex.Challenge, discovery metadataDiscovery) (*oauthex.ProtectedResourceMetadata, error) {
	for _, candidate := range protectedResourceMetadataURLs(resourceMetadataURLFromChallenges(challenges), h.serverURL) {
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate.url, candidate.resource, h.httpClient)
		if err != nil || prm == nil {
			continue
		}
		if len(prm.AuthorizationServers) == 0 {
			return nil, errors.New("protected resource metadata lists no authorization servers")
		}
		return prm, nil
	}
	if h.oauth.Issuer != "" {
		return &oauthex.ProtectedResourceMetadata{
			AuthorizationServers: []string{h.oauth.Issuer},
			Resource:             h.serverURL,
		}, nil
	}
	if discovery == discoveryProactive {
		return nil, errors.New("server did not advertise OAuth protected resource metadata")
	}
	u, err := url.Parse(h.serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server url: %w", err)
	}
	u.Path = ""
	return &oauthex.ProtectedResourceMetadata{
		AuthorizationServers: []string{u.String()},
		Resource:             h.serverURL,
	}, nil
}

type prmCandidate struct {
	url      string
	resource string
}

func protectedResourceMetadataURLs(metadataURL, resourceURL string) []prmCandidate {
	var out []prmCandidate
	if metadataURL != "" {
		out = append(out, prmCandidate{url: metadataURL, resource: resourceURL})
	}
	ru, err := url.Parse(resourceURL)
	if err != nil {
		return out
	}
	mu := *ru
	mu.RawPath, mu.RawQuery, mu.Fragment = "", "", ""
	mu.ForceQuery = false
	const wellKnown = "/.well-known/oauth-protected-resource"
	for _, path := range []string{wellKnown + strings.TrimRight(ru.Path, "/"), wellKnown} {
		mu.Path = path
		candidate := prmCandidate{url: mu.String(), resource: resourceURL}
		if !slices.Contains(out, candidate) {
			out = append(out, candidate)
		}
	}
	origin := (&url.URL{Scheme: ru.Scheme, Host: ru.Host}).String()
	if resourceURL != origin {
		out = append(out, prmCandidate{url: mu.String(), resource: origin})
	}
	return out
}

func bearerChallenges(headers []string) ([]oauthex.Challenge, error) {
	challenges, err := oauthex.ParseWWWAuthenticate(headers)
	if err != nil {
		return nil, fmt.Errorf("parse WWW-Authenticate: %w", err)
	}
	return slices.DeleteFunc(challenges, func(c oauthex.Challenge) bool { return c.Scheme != "bearer" }), nil
}

func resourceMetadataURLFromChallenges(cs []oauthex.Challenge) string {
	for _, c := range cs {
		if u := c.Params["resource_metadata"]; u != "" {
			return u
		}
	}
	return ""
}

func scopesFromChallenges(cs []oauthex.Challenge) []string {
	for _, c := range cs {
		if c.Scheme == "bearer" && c.Params["scope"] != "" {
			return strings.Fields(c.Params["scope"])
		}
	}
	return nil
}

func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func authorizationStateFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("state"))
}
