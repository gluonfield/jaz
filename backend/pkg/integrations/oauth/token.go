package oauth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

var ErrTokenNotFound = errors.New("oauth token not found")

type Token struct {
	AccessToken        string    `json:"access_token"`
	RefreshToken       string    `json:"refresh_token,omitempty"`
	TokenType          string    `json:"token_type,omitempty"`
	Expiry             time.Time `json:"expiry,omitempty"`
	ClientID           string    `json:"client_id"`
	ClientSecret       string    `json:"client_secret,omitempty"`
	ClientSecretEnvVar string    `json:"client_secret_env_var,omitempty"`
	AuthURL            string    `json:"auth_url"`
	TokenURL           string    `json:"token_url"`
	AuthStyle          int       `json:"auth_style"`
	RedirectURL        string    `json:"redirect_url,omitempty"`
	Scopes             []string  `json:"scopes,omitempty"`
	Resource           string    `json:"resource,omitempty"`
}

type Store interface {
	LoadToken(context.Context, string) (Token, bool, error)
	SaveToken(context.Context, string, Token) error
}

type Refresher struct {
	Store      Store
	HTTPClient *http.Client
}

func (r Refresher) TokenSource(ctx context.Context, connectionID string) (oauth2.TokenSource, error) {
	src, err := r.persistentTokenSource(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return src, nil
}

func (r Refresher) persistentTokenSource(ctx context.Context, connectionID string) (*persistingTokenSource, error) {
	if r.Store == nil {
		return nil, ErrTokenNotFound
	}
	stored, ok, err := r.Store.LoadToken(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTokenNotFound
	}
	ctx = r.context(ctx)
	base := stored.config().TokenSource(ctx, stored.oauth2Token())
	return &persistingTokenSource{base: base, store: r.Store, connectionID: connectionID, stored: stored}, nil
}

func (r Refresher) Client(ctx context.Context, connectionID string) (*http.Client, error) {
	src, err := r.TokenSource(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(r.context(ctx), src), nil
}

func (r Refresher) FreshToken(ctx context.Context, connectionID string) (Token, error) {
	src, err := r.persistentTokenSource(ctx, connectionID)
	if err != nil {
		return Token{}, err
	}
	return src.FreshToken()
}

func (r Refresher) context(ctx context.Context) context.Context {
	if r.HTTPClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, r.HTTPClient)
}

func (t Token) config() *oauth2.Config {
	clientSecret := t.ClientSecret
	if clientSecret == "" && strings.TrimSpace(t.ClientSecretEnvVar) != "" {
		clientSecret = os.Getenv(strings.TrimSpace(t.ClientSecretEnvVar))
	}
	return &oauth2.Config{
		ClientID:     t.ClientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   t.AuthURL,
			TokenURL:  t.TokenURL,
			AuthStyle: oauth2.AuthStyle(t.AuthStyle),
		},
		RedirectURL: t.RedirectURL,
		Scopes:      t.Scopes,
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

type persistingTokenSource struct {
	mu           sync.Mutex
	base         oauth2.TokenSource
	store        Store
	connectionID string
	stored       Token
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if tok != nil && !sameToken(p.stored, tok) {
		updated := mergeToken(p.stored, tok)
		if err := p.store.SaveToken(context.Background(), p.connectionID, updated); err != nil {
			return nil, err
		}
		p.stored = updated
	}
	return tok, nil
}

func (p *persistingTokenSource) FreshToken() (Token, error) {
	tok, err := p.Token()
	if err != nil || tok == nil {
		return Token{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stored, nil
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
