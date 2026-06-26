package connections

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type OAuthStart struct {
	AuthURL string `json:"auth_url"`
}

type OAuthStore interface {
	LoadToken(context.Context, string) (integrationoauth.Token, bool, error)
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	SaveOAuthConnection(context.Context, integrationoauth.Token, integrations.Connection) error
}

type OAuthConfig struct {
	Gmail gmailconnector.OAuthClientConfig
}

type OAuthService struct {
	store  OAuthStore
	config OAuthConfig
	mu     sync.Mutex
	states map[string]oauthState
}

type oauthState struct {
	pluginID    string
	verifier    string
	redirectURL string
	expiresAt   time.Time
}

func NewOAuthService(store OAuthStore, config OAuthConfig) *OAuthService {
	return &OAuthService{
		store:  store,
		config: config,
		states: map[string]oauthState{},
	}
}

func (s *OAuthService) Start(ctx context.Context, pluginID, redirectURL string) (OAuthStart, error) {
	if pluginID != gmailconnector.ProviderID {
		return OAuthStart{}, fmt.Errorf("connection plugin %q does not support OAuth yet", pluginID)
	}
	if redirectURL == "" {
		return OAuthStart{}, errors.New("redirect URL is required")
	}
	config, _, err := s.gmailOAuthConfig(ctx, redirectURL)
	if err != nil {
		return OAuthStart{}, err
	}
	state, err := randomOAuthState()
	if err != nil {
		return OAuthStart{}, err
	}
	verifier := oauth2.GenerateVerifier()
	s.mu.Lock()
	s.states[state] = oauthState{
		pluginID:    pluginID,
		verifier:    verifier,
		redirectURL: redirectURL,
		expiresAt:   time.Now().UTC().Add(10 * time.Minute),
	}
	s.mu.Unlock()

	return OAuthStart{AuthURL: config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)}, nil
}

func (s *OAuthService) Callback(ctx context.Context, state, code, failure string) error {
	if failure != "" {
		return fmt.Errorf("Google rejected authorization: %s", failure)
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
	if stored.pluginID != gmailconnector.ProviderID {
		return fmt.Errorf("connection plugin %q does not support callback exchange", stored.pluginID)
	}

	config, credentials, err := s.gmailOAuthConfig(ctx, stored.redirectURL)
	if err != nil {
		return err
	}
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(stored.verifier))
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	profile, err := defaultGmailProfile(ctx, token)
	if err != nil {
		return fmt.Errorf("gmail verification failed: %w", err)
	}
	storedToken := integrationoauth.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		AuthURL:      gmailconnector.OAuthAuthURL,
		TokenURL:     gmailconnector.OAuthTokenURL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
		RedirectURL:  stored.redirectURL,
		Scopes:       gmailconnector.OAuthScopes,
	}
	connection, err := s.gmailConnection(ctx, profile)
	if err != nil {
		return err
	}
	return s.store.SaveOAuthConnection(ctx, storedToken, connection)
}

func defaultGmailProfile(ctx context.Context, token *oauth2.Token) (gmailconnector.Profile, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	return gmailconnector.APIClient{HTTP: client}.Profile(ctx)
}

func (s *OAuthService) gmailConnection(ctx context.Context, profile gmailconnector.Profile) (integrations.Connection, error) {
	connections, err := s.store.ListConnections(ctx, gmailconnector.ProviderID)
	if err != nil {
		return integrations.Connection{}, err
	}
	accountID := strings.TrimSpace(profile.EmailAddress)
	for _, connection := range connections {
		if strings.EqualFold(strings.TrimSpace(connection.AccountID), accountID) {
			connection.AccountID = accountID
			connection.AccountName = accountID
			if connection.Alias == "" {
				connection.Alias = integrations.DefaultAlias(accountID, accountID)
			}
			connection.Scopes = gmailconnector.OAuthScopes
			return connection, nil
		}
	}
	id, err := gmailconnector.ConnectionID(accountID)
	if err != nil {
		return integrations.Connection{}, err
	}
	return integrations.Connection{
		ID:          id,
		Provider:    gmailconnector.ProviderID,
		AccountID:   accountID,
		AccountName: accountID,
		Alias:       integrations.DefaultAlias(accountID, accountID),
		Scopes:      gmailconnector.OAuthScopes,
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

func (s *OAuthService) gmailOAuthConfig(ctx context.Context, redirectURL string) (*oauth2.Config, gmailconnector.OAuthClientCredentials, error) {
	credentials, err := s.gmailCredentials(ctx)
	if err != nil {
		return nil, gmailconnector.OAuthClientCredentials{}, err
	}
	return &oauth2.Config{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   gmailconnector.OAuthAuthURL,
			TokenURL:  gmailconnector.OAuthTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      gmailconnector.OAuthScopes,
	}, credentials, nil
}

func (s *OAuthService) gmailCredentials(ctx context.Context) (gmailconnector.OAuthClientCredentials, error) {
	credentials, err := s.config.Gmail.Credentials()
	if err == nil {
		return credentials, nil
	}
	if strings.TrimSpace(s.config.Gmail.ClientID) != "" || strings.TrimSpace(s.config.Gmail.ClientSecret) != "" {
		return gmailconnector.OAuthClientCredentials{}, err
	}
	stored, ok, loadErr := s.storedGmailCredentials(ctx)
	if loadErr != nil {
		return gmailconnector.OAuthClientCredentials{}, loadErr
	}
	if ok {
		return stored, nil
	}
	return gmailconnector.OAuthClientCredentials{}, err
}

func (s *OAuthService) storedGmailCredentials(ctx context.Context) (gmailconnector.OAuthClientCredentials, bool, error) {
	connections, err := s.store.ListConnections(ctx, gmailconnector.ProviderID)
	if err != nil {
		return gmailconnector.OAuthClientCredentials{}, false, err
	}
	for _, connection := range connections {
		token, ok, err := s.store.LoadToken(ctx, connection.ID)
		if err != nil {
			return gmailconnector.OAuthClientCredentials{}, false, err
		}
		if !ok {
			continue
		}
		id := strings.TrimSpace(token.ClientID)
		secret := strings.TrimSpace(token.ClientSecret)
		if id != "" && secret != "" {
			return gmailconnector.OAuthClientCredentials{ClientID: id, ClientSecret: secret}, true, nil
		}
	}
	return gmailconnector.OAuthClientCredentials{}, false, nil
}

func randomOAuthState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
