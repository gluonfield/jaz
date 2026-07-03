package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	slackconnector "github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type OAuthStart struct {
	AuthURL string `json:"auth_url"`
}

type OAuthStore interface {
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	SaveOAuthConnection(context.Context, integrationoauth.Token, integrations.Connection) error
}

// DefaultOAuthRedirectBroker is the hosted HTTPS endpoint that bounces provider
// callbacks to a desktop Jaz's local loopback, for providers (e.g. Slack) that
// reject loopback redirect URIs.
const DefaultOAuthRedirectBroker = "https://jaz.chat/oauth/callback"

type OAuthConfig struct {
	Gmail             gmailconnector.OAuthClientConfig
	Slack             slackconnector.OAuthClientConfig
	RedirectBrokerURL string
}

// oauthProvider adapts a connector's OAuth primitives to the generic flow. The
// service owns state, PKCE, connection dedup, and persistence; a provider only
// builds its consent URL and exchanges a code for a token plus account identity.
type oauthProvider interface {
	id() string
	// usesBroker reports whether the provider rejects loopback redirect URIs and
	// must go through the hosted HTTPS redirect broker.
	usesBroker() bool
	authCodeURL(redirectURL, state, verifier string) (string, error)
	exchange(ctx context.Context, httpClient *http.Client, redirectURL, code, verifier string) (integrationoauth.Token, oauthIdentity, error)
}

type oauthIdentity struct {
	accountID   string
	accountName string
	scopes      []string
}

type OAuthService struct {
	store      OAuthStore
	providers  map[string]oauthProvider
	brokerURL  string
	httpClient *http.Client
	mu         sync.Mutex
	states     map[string]oauthState
}

type oauthState struct {
	pluginID    string
	verifier    string
	redirectURL string
	expiresAt   time.Time
}

func NewOAuthService(store OAuthStore, config OAuthConfig) *OAuthService {
	providers := map[string]oauthProvider{}
	for _, p := range []oauthProvider{
		gmailOAuthProvider{config: config.Gmail},
		slackOAuthProvider{config: config.Slack},
	} {
		providers[p.id()] = p
	}
	return &OAuthService{
		store:      store,
		providers:  providers,
		brokerURL:  strings.TrimRight(strings.TrimSpace(config.RedirectBrokerURL), "/"),
		httpClient: http.DefaultClient,
		states:     map[string]oauthState{},
	}
}

func (s *OAuthService) Start(_ context.Context, pluginID, localCallbackURL string) (OAuthStart, error) {
	if localCallbackURL == "" {
		return OAuthStart{}, errors.New("redirect URL is required")
	}
	provider, ok := s.providers[pluginID]
	if !ok {
		return OAuthStart{}, fmt.Errorf("connection plugin %q does not support OAuth yet", pluginID)
	}
	nonce, err := randomOAuthState()
	if err != nil {
		return OAuthStart{}, err
	}
	verifier := oauth2.GenerateVerifier()

	// Providers that reject loopback callbacks are sent through the hosted broker,
	// which bounces the response back to localCallbackURL (carried in state).
	redirectURL, state := localCallbackURL, nonce
	if provider.usesBroker() && s.brokerURL != "" && isLoopbackHTTP(localCallbackURL) {
		redirectURL = s.brokerURL
		state = encodeBrokerState(nonce, localCallbackURL)
	}

	authURL, err := provider.authCodeURL(redirectURL, state, verifier)
	if err != nil {
		return OAuthStart{}, err
	}
	s.mu.Lock()
	s.states[state] = oauthState{
		pluginID:    pluginID,
		verifier:    verifier,
		redirectURL: redirectURL,
		expiresAt:   time.Now().UTC().Add(10 * time.Minute),
	}
	s.mu.Unlock()
	return OAuthStart{AuthURL: authURL}, nil
}

func (s *OAuthService) Callback(ctx context.Context, state, code, failure string) error {
	if failure != "" {
		return fmt.Errorf("authorization was rejected: %s", failure)
	}
	if code == "" {
		return errors.New("authorization returned no code")
	}
	stored, ok := s.takeState(state)
	if !ok {
		return errors.New("authorization state expired or was not started by Jaz")
	}
	if time.Now().UTC().After(stored.expiresAt) {
		return errors.New("authorization state expired")
	}
	provider, ok := s.providers[stored.pluginID]
	if !ok {
		return fmt.Errorf("connection plugin %q does not support callback exchange", stored.pluginID)
	}
	token, identity, err := provider.exchange(ctx, s.httpClient, stored.redirectURL, code, stored.verifier)
	if err != nil {
		return err
	}
	connection, err := s.upsertConnection(ctx, provider.id(), identity)
	if err != nil {
		return err
	}
	return s.store.SaveOAuthConnection(ctx, token, connection)
}

func (s *OAuthService) upsertConnection(ctx context.Context, provider string, identity oauthIdentity) (integrations.Connection, error) {
	accountID := strings.TrimSpace(identity.accountID)
	accountName := identity.accountName
	if accountName == "" {
		accountName = accountID
	}
	connections, err := s.store.ListConnections(ctx, provider)
	if err != nil {
		return integrations.Connection{}, err
	}
	for _, connection := range connections {
		if strings.EqualFold(strings.TrimSpace(connection.AccountID), accountID) {
			connection.AccountID = accountID
			connection.AccountName = accountName
			if connection.Alias == "" {
				connection.Alias = integrations.DefaultAlias(accountName, accountID)
			}
			connection.Scopes = identity.scopes
			return connection, nil
		}
	}
	id, err := integrations.ConnectionID(provider, accountID)
	if err != nil {
		return integrations.Connection{}, err
	}
	return integrations.Connection{
		ID:          id,
		Provider:    provider,
		AccountID:   accountID,
		AccountName: accountName,
		Alias:       integrations.DefaultAlias(accountName, accountID),
		Scopes:      identity.scopes,
	}, nil
}

func (s *OAuthService) takeState(state string) (oauthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	return stored, ok
}

func randomOAuthState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// encodeBrokerState packs the loopback callback into the OAuth state so the
// hosted broker knows where to bounce the response. The broker validates it is
// a loopback address before redirecting.
func encodeBrokerState(nonce, localCallbackURL string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(nonce + "|" + localCallbackURL))
}

func isLoopbackHTTP(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
}
