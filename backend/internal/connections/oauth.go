package connections

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type OAuthStart struct {
	AuthURL string `json:"auth_url"`
}

type OAuthService struct {
	tokens integrationoauth.Store
	mu     sync.Mutex
	states map[string]oauthState
}

type oauthState struct {
	pluginID    string
	verifier    string
	redirectURL string
	expiresAt   time.Time
}

func NewOAuthService(tokens integrationoauth.Store) *OAuthService {
	return &OAuthService{tokens: tokens, states: map[string]oauthState{}}
}

func (s *OAuthService) Start(_ context.Context, pluginID, redirectURL string) (OAuthStart, error) {
	if pluginID != gmailconnector.ProviderID {
		return OAuthStart{}, fmt.Errorf("connection plugin %q does not support OAuth yet", pluginID)
	}
	if redirectURL == "" {
		return OAuthStart{}, errors.New("redirect URL is required")
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

	return OAuthStart{AuthURL: gmailOAuthConfig(redirectURL).AuthCodeURL(
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

	token, err := gmailOAuthConfig(stored.redirectURL).Exchange(ctx, code, oauth2.VerifierOption(stored.verifier))
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	return s.tokens.SaveToken(ctx, gmailconnector.OAuthConnectionID, integrationoauth.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		ClientID:     gmailconnector.OAuthClientID,
		ClientSecret: gmailconnector.OAuthClientSecret,
		AuthURL:      gmailconnector.OAuthAuthURL,
		TokenURL:     gmailconnector.OAuthTokenURL,
		AuthStyle:    int(oauth2.AuthStyleInParams),
		RedirectURL:  stored.redirectURL,
		Scopes:       gmailconnector.OAuthScopes,
	})
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

func gmailOAuthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     gmailconnector.OAuthClientID,
		ClientSecret: gmailconnector.OAuthClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   gmailconnector.OAuthAuthURL,
			TokenURL:  gmailconnector.OAuthTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURL,
		Scopes:      gmailconnector.OAuthScopes,
	}
}

func randomOAuthState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
