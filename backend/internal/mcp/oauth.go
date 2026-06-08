package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
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
	store      mcpconfig.OAuthTokenStore
	httpClient *http.Client
	log        *log.Logger

	interactive bool
	redirectURL string
	fetch       codeFetcher

	mu            sync.Mutex
	src           oauth2.TokenSource
	authRequested bool
}

// codeFetcher directs the user through the authorization URL (e.g. opens a browser)
// and returns the authorization code and state once the redirect arrives.
type codeFetcher func(ctx context.Context, authURL string) (code, state string, err error)

func newOAuthHandler(server mcpconfig.Server, store mcpconfig.OAuthTokenStore, httpClient *http.Client, logger *log.Logger) *oauthHandler {
	return &oauthHandler{
		serverID:   server.ID,
		serverURL:  server.URL,
		store:      store,
		httpClient: httpClient,
		log:        logger,
	}
}

func (h *oauthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.src != nil {
		return h.src, nil
	}
	tok, ok, err := h.store.LoadMCPOAuthToken(h.serverID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	cfg := configFromStored(tok)
	base := cfg.TokenSource(h.clientContext(), oauth2Token(tok))
	h.src = &persistingTokenSource{base: base, handler: h, stored: tok, last: tok.AccessToken}
	return h.src, nil
}

func (h *oauthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if !h.interactive {
		h.mu.Lock()
		h.authRequested = true
		h.mu.Unlock()
		return errNeedsAuthorization
	}

	tok, err := h.runAuthorization(ctx, resp)
	if err != nil {
		return err
	}
	if err := h.store.SaveMCPOAuthToken(h.serverID, tok); err != nil {
		return fmt.Errorf("persist token: %w", err)
	}
	h.mu.Lock()
	h.src = nil // force TokenSource to rebuild from the freshly stored token
	h.mu.Unlock()
	return nil
}

// needsAuthorization reports whether a failed connect was due to the server
// requiring an authorization the non-interactive handler could not provide.
func (h *oauthHandler) needsAuthorization() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.authRequested
}

func (h *oauthHandler) clientContext() context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, h.httpClient)
}

// runAuthorization performs the OAuth 2.0 authorization-code flow with PKCE and
// dynamic client registration, following the MCP authorization spec. The logic
// mirrors the go-sdk's AuthorizationCodeHandler but captures the resolved client
// and endpoints so the token can be refreshed across restarts.
func (h *oauthHandler) runAuthorization(ctx context.Context, resp *http.Response) (mcpconfig.OAuthToken, error) {
	var zero mcpconfig.OAuthToken
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")])
	if err != nil {
		return zero, fmt.Errorf("parse WWW-Authenticate: %w", err)
	}

	prm, err := h.protectedResourceMetadata(ctx, challenges)
	if err != nil {
		return zero, err
	}
	if len(prm.AuthorizationServers) == 0 {
		return zero, errors.New("server advertised no authorization servers")
	}

	asm, err := auth.GetAuthServerMetadata(ctx, prm.AuthorizationServers[0], h.httpClient)
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
	if asm.RegistrationEndpoint == "" {
		return zero, errors.New("authorization server does not support dynamic client registration")
	}

	reg, err := oauthex.RegisterClient(ctx, asm.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{h.redirectURL},
		ClientName:              "Jaz",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	}, h.httpClient)
	if err != nil {
		return zero, fmt.Errorf("register client: %w", err)
	}

	scopes := scopesFromChallenges(challenges)
	if len(scopes) == 0 {
		scopes = prm.ScopesSupported
	}
	authStyle := oauth2.AuthStyleInParams // public client (token_endpoint_auth_method=none)

	cfg := &oauth2.Config{
		ClientID:     reg.ClientID,
		ClientSecret: reg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   asm.AuthorizationEndpoint,
			TokenURL:  asm.TokenEndpoint,
			AuthStyle: authStyle,
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

	return mcpconfig.OAuthToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     reg.ClientID,
		ClientSecret: reg.ClientSecret,
		AuthURL:      asm.AuthorizationEndpoint,
		TokenURL:     asm.TokenEndpoint,
		AuthStyle:    int(authStyle),
		RedirectURL:  h.redirectURL,
		Scopes:       scopes,
		Resource:     prm.Resource,
	}, nil
}

func (h *oauthHandler) exchangeContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, h.httpClient)
}

// protectedResourceMetadata discovers the protected resource metadata, trying the
// URL from the WWW-Authenticate challenge, then the well-known locations, and
// finally falling back to treating the server origin as the authorization server.
func (h *oauthHandler) protectedResourceMetadata(ctx context.Context, challenges []oauthex.Challenge) (*oauthex.ProtectedResourceMetadata, error) {
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

// persistingTokenSource saves the token back to the store whenever the underlying
// source mints a new access token (e.g. after a refresh).
type persistingTokenSource struct {
	base    oauth2.TokenSource
	handler *oauthHandler
	stored  mcpconfig.OAuthToken
	last    string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if tok != nil && tok.AccessToken != p.last {
		p.last = tok.AccessToken
		updated := p.stored
		updated.AccessToken = tok.AccessToken
		updated.TokenType = tok.TokenType
		updated.Expiry = tok.Expiry
		if tok.RefreshToken != "" {
			updated.RefreshToken = tok.RefreshToken
		}
		p.stored = updated
		if err := p.handler.store.SaveMCPOAuthToken(p.handler.serverID, updated); err != nil && p.handler.log != nil {
			p.handler.log.Warn("persist refreshed mcp token failed", "server", p.handler.serverID, "error", err)
		}
	}
	return tok, nil
}

func configFromStored(tok mcpconfig.OAuthToken) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     tok.ClientID,
		ClientSecret: tok.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   tok.AuthURL,
			TokenURL:  tok.TokenURL,
			AuthStyle: oauth2.AuthStyle(tok.AuthStyle),
		},
		RedirectURL: tok.RedirectURL,
		Scopes:      tok.Scopes,
	}
}

func oauth2Token(tok mcpconfig.OAuthToken) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  tok.AccessToken,
		TokenType:    tok.TokenType,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
	}
}

func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
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
