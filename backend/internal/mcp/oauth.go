package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/oauth2"
)

// errNeedsAuthorization is returned by a non-interactive handler when the server
// demands authorization but we have no usable token. It lets the manager mark the
// server as needing the user to sign in rather than reporting a generic error.
var errNeedsAuthorization = errors.New("authorization required")

// oauthHandler implements auth.OAuthHandler for a single MCP server. It is backed
// by the persistent token store: TokenSource serves stored (refreshing) tokens, and
// Authorize — only when interactive — runs the browser authorization-code flow and
// persists the result.
type oauthHandler struct {
	serverID   string
	serverURL  string
	oauth      mcpconfig.OAuthConfig
	store      integrationoauth.Store
	httpClient *http.Client

	redirectURL string
	fetch       codeFetcher

	mu    sync.Mutex
	src   oauth2.TokenSource
	mode  oauthMode
	state authorizationState
}

// codeFetcher directs the user through the authorization URL (e.g. opens a browser)
// and returns the authorization code and state once the redirect arrives.
type codeFetcher func(ctx context.Context, authURL string) (code, state string, err error)

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

func newOAuthHandler(server mcpconfig.Server, store integrationoauth.Store, httpClient *http.Client) *oauthHandler {
	return &oauthHandler{
		serverID:   server.ID,
		serverURL:  server.URL,
		oauth:      server.OAuth,
		store:      store,
		httpClient: httpClient,
		mode:       oauthModeBackground,
		state:      authorizationIdle,
	}
}

func (h *oauthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.src != nil {
		return h.src, nil
	}
	src, err := (integrationoauth.Refresher{
		Store:        h.store,
		HTTPClient:   h.httpClient,
		ClientConfig: h.refreshClientConfig,
	}).TokenSource(ctx, mcpconfig.OAuthConnectionID(h.serverID))
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.src = src
	return h.src, nil
}

func (h *oauthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if h.mode != oauthModeInteractive {
		h.markAuthorizationRequired()
		return errNeedsAuthorization
	}

	tok, err := h.runAuthorization(ctx, resp)
	if err != nil {
		return err
	}
	return h.saveToken(ctx, tok)
}

func (h *oauthHandler) AuthorizeFromMetadata(ctx context.Context) error {
	if h.mode != oauthModeInteractive {
		h.markAuthorizationRequired()
		return errNeedsAuthorization
	}
	tok, err := h.runAuthorizationWithChallenges(ctx, nil, discoveryProactive)
	if err != nil {
		return err
	}
	return h.saveToken(ctx, tok)
}

func (h *oauthHandler) saveToken(ctx context.Context, tok integrationoauth.Token) error {
	if err := h.store.SaveToken(ctx, mcpconfig.OAuthConnectionID(h.serverID), tok); err != nil {
		return fmt.Errorf("persist token: %w", err)
	}
	h.mu.Lock()
	h.src = nil
	h.state = authorizationComplete
	h.mu.Unlock()
	return nil
}

// needsAuthorization reports whether a failed connect was due to the server
// requiring an authorization the non-interactive handler could not provide.
func (h *oauthHandler) needsAuthorization() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state == authorizationRequired
}

func (h *oauthHandler) didAuthorize() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state == authorizationComplete
}

func (h *oauthHandler) markAuthorizationRequired() {
	h.mu.Lock()
	h.state = authorizationRequired
	h.mu.Unlock()
}

// runAuthorization performs the OAuth 2.0 authorization-code flow with PKCE.
// It uses configured client credentials when present and otherwise follows MCP
// dynamic client registration.
func (h *oauthHandler) runAuthorization(ctx context.Context, resp *http.Response) (integrationoauth.Token, error) {
	var zero integrationoauth.Token
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")])
	if err != nil {
		return zero, fmt.Errorf("parse WWW-Authenticate: %w", err)
	}
	return h.runAuthorizationWithChallenges(ctx, challenges, discoveryFromChallenge)
}

func (h *oauthHandler) runAuthorizationWithChallenges(ctx context.Context, challenges []oauthex.Challenge, discovery metadataDiscovery) (integrationoauth.Token, error) {
	var zero integrationoauth.Token
	prm, err := h.protectedResourceMetadata(ctx, challenges, discovery)
	if err != nil {
		return zero, err
	}
	if len(prm.AuthorizationServers) == 0 {
		return zero, errors.New("server advertised no authorization servers")
	}

	asm, err := h.authorizationServerMetadata(ctx, prm.AuthorizationServers[0])
	if err != nil {
		return zero, fmt.Errorf("authorization server metadata: %w", err)
	}
	if asm == nil {
		base := strings.TrimRight(prm.AuthorizationServers[0], "/")
		asm = &oauthex.AuthServerMeta{
			Issuer:                base,
			AuthorizationEndpoint: base + "/authorize",
			TokenEndpoint:         base + "/token",
			RegistrationEndpoint:  base + "/register",
		}
	}

	scopes := scopesFromChallenges(challenges)
	if len(scopes) == 0 {
		scopes = prm.ScopesSupported
	}
	clientConfig, err := h.authorizationClientConfig(ctx, asm)
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
		RedirectURL: h.redirectURL,
		Scopes:      scopes,
	}

	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return zero, err
	}
	authURL := cfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("resource", prm.Resource),
	)

	code, gotState, err := h.fetch(ctx, authURL)
	if err != nil {
		return zero, err
	}
	if gotState != state {
		return zero, errors.New("authorization state mismatch")
	}

	token, err := cfg.Exchange(h.exchangeContext(ctx), code,
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
		ClientID:     storedClientID(clientConfig),
		ClientSecret: dynamicClientSecret(clientConfig),
		AuthURL:      asm.AuthorizationEndpoint,
		TokenURL:     asm.TokenEndpoint,
		AuthStyle:    int(clientConfig.authStyle),
		RedirectURL:  h.redirectURL,
		Scopes:       scopes,
		Resource:     prm.Resource,
	}, nil
}

func (h *oauthHandler) authorizationServerMetadata(ctx context.Context, issuerURL string) (*oauthex.AuthServerMeta, error) {
	asm, err := auth.GetAuthServerMetadata(ctx, issuerURL, h.httpClient)
	if err == nil {
		return asm, nil
	}
	if !isAuthServerIssuerMismatch(err) {
		return nil, err
	}
	// Slack's MCP resource advertises mcp.slack.com as its authorization server,
	// while the fetched metadata has slack.com as the canonical issuer. Keep the
	// SDK's normal strict path first, then only accept same-site canonical aliases.
	asm, fallbackErr := h.authorizationServerMetadataWithCanonicalIssuer(ctx, issuerURL)
	if fallbackErr != nil {
		return nil, fmt.Errorf("%w; canonical issuer fallback failed: %v", err, fallbackErr)
	}
	if asm == nil {
		return nil, err
	}
	return asm, nil
}

func (h *oauthHandler) authorizationServerMetadataWithCanonicalIssuer(ctx context.Context, issuerURL string) (*oauthex.AuthServerMeta, error) {
	for _, metadataURL := range authorizationServerMetadataURLs(issuerURL) {
		metadataIssuer, err := h.authorizationServerMetadataIssuer(ctx, metadataURL)
		if err != nil {
			return nil, err
		}
		if metadataIssuer == "" {
			continue
		}
		if !authorizationServerIssuerTrusted(issuerURL, metadataIssuer) {
			return nil, fmt.Errorf("metadata issuer %q is not trusted for issuer URL %q", metadataIssuer, issuerURL)
		}
		asm, err := oauthex.GetAuthServerMeta(ctx, metadataURL, metadataIssuer, h.httpClient)
		if err != nil {
			return nil, err
		}
		if asm != nil {
			return asm, nil
		}
	}
	return nil, nil
}

func (h *oauthHandler) authorizationServerMetadataIssuer(ctx context.Context, metadataURL string) (string, error) {
	if err := checkAuthMetadataURL(metadataURL); err != nil {
		return "", err
	}
	client := h.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if 400 <= resp.StatusCode && resp.StatusCode < 500 {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s: %s", metadataURL, resp.Status)
	}
	var body struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("decode authorization server metadata: %w", err)
	}
	return strings.TrimSpace(body.Issuer), nil
}

func isAuthServerIssuerMismatch(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "metadata issuer") && strings.Contains(msg, "does not match issuer URL")
}

func authorizationServerMetadataURLs(issuerURL string) []string {
	var urls []string
	baseURL, err := url.Parse(issuerURL)
	if err != nil {
		return nil
	}
	if baseURL.Path == "" {
		baseURL.Path = "/.well-known/oauth-authorization-server"
		urls = append(urls, baseURL.String())
		baseURL.Path = "/.well-known/openid-configuration"
		urls = append(urls, baseURL.String())
		return urls
	}

	originalPath := baseURL.Path
	baseURL.Path = "/.well-known/oauth-authorization-server/" + strings.TrimLeft(originalPath, "/")
	urls = append(urls, baseURL.String())
	baseURL.Path = "/.well-known/openid-configuration/" + strings.TrimLeft(originalPath, "/")
	urls = append(urls, baseURL.String())
	baseURL.Path = "/" + strings.Trim(originalPath, "/") + "/.well-known/openid-configuration"
	urls = append(urls, baseURL.String())
	return urls
}

func authorizationServerIssuerTrusted(issuerURL, metadataIssuer string) bool {
	issuer, ok := parseAliasableIssuerURL(issuerURL)
	if !ok {
		return false
	}
	metadata, ok := parseAliasableIssuerURL(metadataIssuer)
	if !ok {
		return false
	}
	if strings.EqualFold(issuer.Host, metadata.Host) && cleanIssuerPath(issuer.Path) == cleanIssuerPath(metadata.Path) {
		return true
	}
	if cleanIssuerPath(issuer.Path) != "" || cleanIssuerPath(metadata.Path) != "" {
		return false
	}
	if issuer.Port() != "" || metadata.Port() != "" {
		return false
	}
	issuerHost := strings.ToLower(issuer.Hostname())
	metadataHost := strings.ToLower(metadata.Hostname())
	if !strings.HasSuffix(issuerHost, "."+metadataHost) {
		return false
	}
	issuerDomain, err := publicsuffix.EffectiveTLDPlusOne(issuerHost)
	if err != nil {
		return false
	}
	metadataDomain, err := publicsuffix.EffectiveTLDPlusOne(metadataHost)
	if err != nil {
		return false
	}
	return issuerDomain == metadataDomain
}

func parseAliasableIssuerURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, false
	}
	return u, true
}

func cleanIssuerPath(path string) string {
	return strings.TrimRight(path, "/")
}

func checkAuthMetadataURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("authorization server metadata URL must use https or loopback http: %s", raw)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *oauthHandler) authorizationClientConfig(ctx context.Context, asm *oauthex.AuthServerMeta) (oauthClientConfig, error) {
	if h.oauth.ClientID != "" {
		clientSecret, err := h.staticClientSecret()
		if err != nil {
			return oauthClientConfig{}, err
		}
		authStyle := oauth2.AuthStyleInParams
		if clientSecret != "" {
			authStyle = oauth2.AuthStyleAutoDetect
		}
		return oauthClientConfig{clientID: h.oauth.ClientID, clientSecret: clientSecret, authStyle: authStyle, static: true}, nil
	}
	if asm.RegistrationEndpoint == "" {
		return oauthClientConfig{}, errors.New("authorization server does not support dynamic client registration; configure an OAuth client ID")
	}
	reg, err := oauthex.RegisterClient(ctx, asm.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{h.redirectURL},
		ClientName:              "Jaz",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	}, h.httpClient)
	if err != nil {
		return oauthClientConfig{}, fmt.Errorf("register client: %w", err)
	}
	return oauthClientConfig{clientID: reg.ClientID, clientSecret: reg.ClientSecret, authStyle: oauth2.AuthStyleInParams}, nil
}

func (h *oauthHandler) refreshClientConfig(ctx context.Context, token integrationoauth.Token) (integrationoauth.ClientConfig, error) {
	if h.oauth.ClientID != "" {
		clientSecret, err := h.staticClientSecret()
		if err != nil {
			return integrationoauth.ClientConfig{}, err
		}
		authStyle := oauth2.AuthStyleInParams
		if clientSecret != "" {
			authStyle = oauth2.AuthStyleAutoDetect
		}
		return integrationoauth.ClientConfig{
			ClientID:     h.oauth.ClientID,
			ClientSecret: clientSecret,
			AuthURL:      token.AuthURL,
			TokenURL:     token.TokenURL,
			AuthStyle:    authStyle,
			RedirectURL:  token.RedirectURL,
			Scopes:       token.Scopes,
		}, nil
	}
	return integrationoauth.ClientConfig{
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AuthURL:      token.AuthURL,
		TokenURL:     token.TokenURL,
		AuthStyle:    oauth2.AuthStyle(token.AuthStyle),
		RedirectURL:  token.RedirectURL,
		Scopes:       token.Scopes,
	}, nil
}

func (h *oauthHandler) staticClientSecret() (string, error) {
	if h.oauth.ClientSecretEnvVar == "" {
		return "", nil
	}
	clientSecret := os.Getenv(h.oauth.ClientSecretEnvVar)
	if clientSecret == "" {
		return "", fmt.Errorf("environment variable %s is not set", h.oauth.ClientSecretEnvVar)
	}
	return clientSecret, nil
}

func dynamicClientSecret(client oauthClientConfig) string {
	if client.static {
		return ""
	}
	return client.clientSecret
}

func storedClientID(client oauthClientConfig) string {
	if client.static {
		return ""
	}
	return client.clientID
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
	mu.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(ru.Path, "/")
	out = append(out, prmCandidate{url: mu.String(), resource: resourceURL})
	mu.Path = "/.well-known/oauth-protected-resource"
	ru.Path = ""
	out = append(out, prmCandidate{url: mu.String(), resource: ru.String()})
	return out
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

// loopbackReceiver runs a localhost HTTP server that captures the OAuth redirect.
type loopbackReceiver struct {
	srv         *http.Server
	redirectURL string
	result      chan loopbackResult
}

type loopbackResult struct {
	code  string
	state string
	err   string
}

func newLoopbackReceiver() (*loopbackReceiver, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	lb := &loopbackReceiver{
		redirectURL: fmt.Sprintf("http://%s/callback", ln.Addr().String()),
		result:      make(chan loopbackResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		select {
		case lb.result <- loopbackResult{code: q.Get("code"), state: q.Get("state"), err: q.Get("error")}:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, callbackHTML)
	})
	lb.srv = &http.Server{Handler: mux}
	go func() { _ = lb.srv.Serve(ln) }()
	return lb, nil
}

func (lb *loopbackReceiver) fetch(ctx context.Context, authURL string) (string, string, error) {
	if err := openBrowser(authURL); err != nil {
		// Not fatal: the user can still complete sign-in if the URL is surfaced elsewhere.
		fmt.Printf("Open this URL to authorize the MCP server:\n%s\n", authURL)
	}
	select {
	case res := <-lb.result:
		if res.err != "" {
			return "", "", fmt.Errorf("authorization failed: %s", res.err)
		}
		if res.code == "" {
			return "", "", errors.New("authorization returned no code")
		}
		return res.code, res.state, nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

func (lb *loopbackReceiver) close() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = lb.srv.Shutdown(shutdownCtx)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

const callbackHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Jaz</title></head>
<body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;color:#333">
<div style="text-align:center"><h2 style="font-weight:600">You're signed in</h2>
<p style="color:#666">You can close this tab and return to Jaz.</p></div></body></html>`
