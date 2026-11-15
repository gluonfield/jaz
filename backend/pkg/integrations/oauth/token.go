package oauth

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

var ErrTokenNotFound = errors.New("oauth token not found")

type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	Issuer       string    `json:"issuer,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty"`
	AuthURL      string    `json:"auth_url"`
	TokenURL     string    `json:"token_url"`
	AuthStyle    int       `json:"auth_style"`
	RedirectURL  string    `json:"redirect_url,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	Resource     string    `json:"resource,omitempty"`
}

type Store interface {
	LoadToken(context.Context, string) (Token, bool, error)
	SaveToken(context.Context, string, Token) error
}

type ClientConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	AuthStyle    oauth2.AuthStyle
	RedirectURL  string
	Scopes       []string
}

type ClientConfigResolver func(context.Context, Token) (ClientConfig, error)

type Refresher struct {
	Store        Store
	HTTPClient   *http.Client
	ClientConfig ClientConfigResolver
}

func (r Refresher) TokenSource(ctx context.Context, connectionID string) (oauth2.TokenSource, error) {
	if r.Store == nil {
		return nil, ErrTokenNotFound
	}
	_, ok, err := r.Store.LoadToken(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTokenNotFound
	}
	return &persistingTokenSource{ctx: r.context(ctx), refresher: r, connectionID: connectionID}, nil
}

func (r Refresher) Client(ctx context.Context, connectionID string) (*http.Client, error) {
	src, err := r.TokenSource(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	client := *oauth2.NewClient(r.context(ctx), nil)
	client.Transport = &oauth2.Transport{Base: client.Transport, Source: src}
	return &client, nil
}

func (r Refresher) context(ctx context.Context) context.Context {
	if r.HTTPClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, r.HTTPClient)
}

func (r Refresher) clientConfig(ctx context.Context, token Token) (ClientConfig, error) {
	if r.ClientConfig != nil {
		return r.ClientConfig(ctx, token)
	}
	return ClientConfig{
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AuthURL:      token.AuthURL,
		TokenURL:     token.TokenURL,
		AuthStyle:    oauth2.AuthStyle(token.AuthStyle),
		RedirectURL:  token.RedirectURL,
		Scopes:       token.Scopes,
	}, nil
}

func (c ClientConfig) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   c.AuthURL,
			TokenURL:  c.TokenURL,
			AuthStyle: c.AuthStyle,
		},
		RedirectURL: c.RedirectURL,
		Scopes:      c.Scopes,
	}
}

func (t Token) oauth2Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// refreshLocks holds one lock per connection for every token source in the
// process. Providers may rotate refresh tokens and revoke the grant when a
// rotated one is presented again, so each refresh must start from the stored token.
var refreshLocks sync.Map

// refreshTimeout bounds how long a refresh holds its connection's lock.
const refreshTimeout = 30 * time.Second

// LockToken serializes grant replacement and removal with refresh.
func LockToken(connectionID string) func() {
	lock, _ := refreshLocks.LoadOrStore(connectionID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

type persistingTokenSource struct {
	ctx          context.Context
	refresher    Refresher
	connectionID string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	unlock := LockToken(p.connectionID)
	defer unlock()
	ctx, cancel := context.WithTimeout(p.ctx, refreshTimeout)
	defer cancel()
	stored, ok, err := p.refresher.Store.LoadToken(ctx, p.connectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTokenNotFound
	}
	if tok := stored.oauth2Token(); tok.Valid() {
		return tok, nil
	}
	config, err := p.refresher.clientConfig(ctx, stored)
	if err != nil {
		return nil, err
	}
	if stored.Resource != "" {
		client := *oauth2.NewClient(ctx, nil)
		base := client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		client.Transport = resourceTransport{base: base, resource: stored.Resource}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, &client)
	}
	tok, err := config.config().TokenSource(ctx, stored.oauth2Token()).Token()
	if err != nil {
		return nil, err
	}
	if !sameToken(stored, tok) {
		stored = mergeToken(stored, tok)
		if err := p.refresher.Store.SaveToken(context.Background(), p.connectionID, stored); err != nil {
			return nil, err
		}
	}
	return tok, nil
}

func sameToken(stored Token, tok *oauth2.Token) bool {
	if tok == nil {
		return false
	}
	return stored.AccessToken == tok.AccessToken &&
		stored.RefreshToken == tok.RefreshToken &&
		stored.TokenType == tok.TokenType &&
		stored.Expiry.Equal(tok.Expiry)
}

func mergeToken(stored Token, tok *oauth2.Token) Token {
	updated := stored
	updated.AccessToken = tok.AccessToken
	updated.TokenType = tok.TokenType
	updated.Expiry = tok.Expiry
	if tok.RefreshToken != "" {
		updated.RefreshToken = tok.RefreshToken
	}
	return updated
}
