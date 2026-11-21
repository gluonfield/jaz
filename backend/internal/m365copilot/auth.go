package m365copilot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/oauthbroker"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

const (
	loginStateTTL    = 15 * time.Minute
	browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

type browserUserAgentTransport struct {
	base http.RoundTripper
}

func (t browserUserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	if next.Header.Get("User-Agent") == "" {
		next.Header.Set("User-Agent", browserUserAgent)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(next)
}

func newOutboundHTTPClient() *http.Client {
	return &http.Client{
		Transport: browserUserAgentTransport{base: http.DefaultTransport},
	}
}

type TokenStore interface {
	integrationoauth.Store
	DeleteToken(context.Context, string) error
}

type AuthStatus struct {
	Authenticated  bool
	Reason         string
	LoginAvailable bool
	LoginReason    string
}

type LoginSession struct {
	State       string
	AuthURL     string
	RedirectURL string
	Done        <-chan error
}

type pendingLogin struct {
	verifier    string
	redirectURL string
	expiresAt   time.Time
	done        chan error
	once        sync.Once
}

func (p *pendingLogin) finish(err error) {
	p.once.Do(func() {
		p.done <- err
		close(p.done)
	})
}

type AuthService struct {
	store      TokenStore
	cfg        OAuthConfig
	broker     oauthbroker.Broker
	httpClient *http.Client
	guard      *integrationoauth.RefreshGuard

	mu      sync.Mutex
	pending map[string]*pendingLogin
}

func NewAuthService(store TokenStore, cfg OAuthConfig, broker oauthbroker.Broker) *AuthService {
	return &AuthService{
		store:      store,
		cfg:        cfg.Normalize(),
		broker:     broker,
		httpClient: newOutboundHTTPClient(),
		guard:      &integrationoauth.RefreshGuard{},
		pending:    map[string]*pendingLogin{},
	}
}

func (s *AuthService) GraphBaseURL() string {
	return s.cfg.GraphBaseURL
}

func (s *AuthService) HTTPClient() *http.Client {
	return s.httpClient
}

func (s *AuthService) ValidateConfig() error {
	return s.cfg.Validate()
}

func (s *AuthService) Status(ctx context.Context) AuthStatus {
	var status AuthStatus
	if err := s.cfg.ValidateRuntime(); err != nil {
		status.Reason = err.Error()
		status.LoginReason = err.Error()
		return status
	}
	cfgErr := s.cfg.Validate()
	if cfgErr == nil {
		status.LoginAvailable = true
	} else {
		status.LoginReason = cfgErr.Error()
	}

	token, ok, err := s.store.LoadToken(ctx, TokenConnectionID)
	if err != nil {
		status.Reason = fmt.Sprintf("read Microsoft Copilot credentials: %v", err)
		return status
	}
	if ok && tokenUsable(token, s.cfg) {
		status.Authenticated = true
		return status
	}
	if cfgErr != nil {
		status.Reason = cfgErr.Error()
		return status
	}
	status.Reason = "Sign in with a licensed Microsoft 365 Copilot work or school account"
	return status
}

func (s *AuthService) StartLogin(ctx context.Context, localCallbackURL string) (LoginSession, error) {
	if err := s.cfg.Validate(); err != nil {
		return LoginSession{}, err
	}
	localCallbackURL = strings.TrimSpace(localCallbackURL)
	state, err := randomState()
	if err != nil {
		return LoginSession{}, err
	}
	verifier := oauth2.GenerateVerifier()

	redirectURL := s.cfg.RedirectURL
	if redirectURL == "" {
		if localCallbackURL == "" {
			return LoginSession{}, errors.New("OAuth redirect URL is required")
		}
		if s.cfg.UseBroker && s.broker.URL != "" {
			redirectURL, state, err = s.broker.Relay(ctx, localCallbackURL)
			if err != nil {
				return LoginSession{}, err
			}
		} else {
			redirectURL = localCallbackURL
		}
	}

	oauthCfg := s.oauth2Config(redirectURL)
	authURL := oauthCfg.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("response_mode", "query"),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)

	now := time.Now().UTC()
	entry := &pendingLogin{
		verifier:    verifier,
		redirectURL: redirectURL,
		expiresAt:   now.Add(loginStateTTL),
		done:        make(chan error, 1),
	}
	s.mu.Lock()
	for key, item := range s.pending {
		if now.After(item.expiresAt) {
			delete(s.pending, key)
		}
	}
	s.pending[state] = entry
	s.mu.Unlock()

	return LoginSession{
		State:       state,
		AuthURL:     authURL,
		RedirectURL: redirectURL,
		Done:        entry.done,
	}, nil
}

func (s *AuthService) CancelLogin(state string) {
	s.mu.Lock()
	entry, ok := s.pending[state]
	if ok {
		delete(s.pending, state)
	}
	s.mu.Unlock()
	if ok {
		entry.finish(context.Canceled)
	}
}

func (s *AuthService) CompleteCallback(ctx context.Context, state, code, oauthErr, oauthErrDesc string) (retErr error) {
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	oauthErr = strings.TrimSpace(oauthErr)
	oauthErrDesc = strings.TrimSpace(oauthErrDesc)

	entry, ok := s.takePending(state)
	if !ok {
		return errors.New("authorization state expired or was not started by Jaz")
	}
	defer func() {
		entry.finish(retErr)
	}()

	if time.Now().UTC().After(entry.expiresAt) {
		return errors.New("authorization state expired")
	}
	if oauthErr != "" {
		if oauthErrDesc != "" {
			return fmt.Errorf("authorization was rejected (%s): %s", oauthErr, oauthErrDesc)
		}
		return fmt.Errorf("authorization was rejected: %s", oauthErr)
	}
	if code == "" {
		return errors.New("authorization returned no code")
	}
	if err := s.cfg.Validate(); err != nil {
		return err
	}

	oauthCfg := s.oauth2Config(entry.redirectURL)
	tok, err := oauthCfg.Exchange(s.oauth2Context(ctx), code, oauth2.VerifierOption(entry.verifier))
	if err != nil {
		return fmt.Errorf("exchange Microsoft OAuth code: %w", err)
	}
	claims := extractTokenClaims(tok)
	if strings.EqualFold(claims.TenantID, PersonalAccountTenantID) {
		return errors.New("personal Microsoft accounts are not supported; sign in with a licensed Microsoft 365 Copilot work or school account")
	}

	saved := integrationoauth.Token{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
		ClientID:     s.cfg.ClientID,
		Issuer:       claims.Issuer,
		AuthURL:      s.cfg.AuthURL(),
		TokenURL:     s.cfg.TokenURL(),
		AuthStyle:    int(oauth2.AuthStyleInParams),
		RedirectURL:  entry.redirectURL,
		Scopes:       RequiredScopes(),
	}

	unlock := integrationoauth.LockToken(TokenConnectionID)
	defer unlock()
	if err := s.store.SaveToken(ctx, TokenConnectionID, saved); err != nil {
		return fmt.Errorf("save Microsoft Copilot credentials: %w", err)
	}
	s.guard.Reset(TokenConnectionID)
	return nil
}

func (s *AuthService) CompleteFromInput(ctx context.Context, expectedState, rawInput string) error {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return errors.New("a code or callback URL is required")
	}
	if parsed, err := url.Parse(rawInput); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		query := parsed.Query()
		if state := strings.TrimSpace(query.Get("state")); state != "" && state != expectedState {
			return errors.New("authorization state did not match this sign-in session")
		}
		return s.CompleteCallback(ctx, expectedState, query.Get("code"), query.Get("error"), query.Get("error_description"))
	}
	return s.CompleteCallback(ctx, expectedState, rawInput, "", "")
}

func (s *AuthService) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	return (integrationoauth.Refresher{
		Store:        s.store,
		HTTPClient:   s.httpClient,
		ClientConfig: s.refreshClientConfig,
		Guard:        s.guard,
	}).TokenSource(ctx, TokenConnectionID)
}

func (s *AuthService) Disconnect(ctx context.Context) error {
	unlock := integrationoauth.LockToken(TokenConnectionID)
	defer unlock()
	if err := s.store.DeleteToken(ctx, TokenConnectionID); err != nil {
		return err
	}
	s.guard.Reset(TokenConnectionID)
	return nil
}

func (s *AuthService) oauth2Config(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   s.cfg.AuthURL(),
			TokenURL:  s.cfg.TokenURL(),
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      RequiredScopes(),
	}
}

func (s *AuthService) oauth2Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
}

func (s *AuthService) refreshClientConfig(_ context.Context, token integrationoauth.Token) (integrationoauth.ClientConfig, error) {
	if err := s.cfg.ValidateRuntime(); err != nil {
		return integrationoauth.ClientConfig{}, err
	}
	clientID := firstNonEmpty(s.cfg.ClientID, token.ClientID)
	tokenURL := s.cfg.TokenURL()
	authURL := s.cfg.AuthURL()
	if s.cfg.ClientID == "" && token.TokenURL != "" {
		tokenURL = token.TokenURL
		authURL = firstNonEmpty(token.AuthURL, authURL)
	}
	if clientID == "" || tokenURL == "" {
		return integrationoauth.ClientConfig{}, ErrClientIDRequired
	}
	return integrationoauth.ClientConfig{
		ClientID:     clientID,
		ClientSecret: firstNonEmpty(s.cfg.ClientSecret, token.ClientSecret),
		AuthURL:      authURL,
		TokenURL:     tokenURL,
		AuthStyle:    oauth2.AuthStyleInParams,
		RedirectURL:  token.RedirectURL,
		Scopes:       RequiredScopes(),
	}, nil
}

func (s *AuthService) takePending(state string) (*pendingLogin, bool) {
	if state == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.pending[state]
	if ok {
		delete(s.pending, state)
	}
	return entry, ok
}

func tokenUsable(token integrationoauth.Token, cfg OAuthConfig) bool {
	if token.AccessToken != "" && (token.Expiry.IsZero() || token.Expiry.After(time.Now().Add(10*time.Second))) {
		return true
	}
	return token.RefreshToken != "" && firstNonEmpty(cfg.ClientID, token.ClientID) != "" && firstNonEmpty(cfg.TokenURL(), token.TokenURL) != ""
}

type jwtClaims struct {
	TenantID string `json:"tid"`
	Issuer   string `json:"iss"`
}

func extractTokenClaims(tok *oauth2.Token) jwtClaims {
	if raw, ok := tok.Extra("id_token").(string); ok {
		if claims, ok := parseJWTClaims(raw); ok {
			return claims
		}
	}
	claims, _ := parseJWTClaims(tok.AccessToken)
	return claims
}

func parseJWTClaims(raw string) (jwtClaims, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return jwtClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, false
	}
	var claims jwtClaims
	if json.Unmarshal(payload, &claims) != nil {
		return jwtClaims{}, false
	}
	return claims, true
}

func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
