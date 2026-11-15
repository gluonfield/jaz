package connections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

func TestOAuthCallbackReplacementWinsOverInflightRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	approved := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") == "refresh_token" {
			close(started)
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"stale","token_type":"Bearer","expires_in":3600}`))
			return
		}
		if r.FormValue("code") != "approved-code" || r.FormValue("code_verifier") == "" || r.FormValue("client_id") != "new-client" {
			t.Error("authorization exchange did not use current client and PKCE")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"approved","refresh_token":"approved-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer upstream.Close()
	account := integrations.Connection{ID: "fixture:account", Provider: "fixture", AccountID: "account"}
	store := &reconnectOAuthStore{connection: account, tokens: map[string]integrationoauth.Token{account.ID: {AccessToken: "expired", RefreshToken: "old-refresh", ClientID: "old-client", TokenURL: upstream.URL, AuthStyle: int(oauth2.AuthStyleInParams), Expiry: time.Now().Add(-time.Hour)}}}
	source, err := (integrationoauth.Refresher{Store: store}).TokenSource(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := NewOAuthService(store, OAuthConfig{})
	service.providers = map[string]oauthProvider{"fixture": googleOAuthProvider{providerID: "fixture", config: googleconnector.OAuthClientConfig{ClientID: "new-client", ClientSecret: "fixture-secret"}, verify: func(context.Context, *http.Client) (oauthIdentity, error) {
		close(approved)
		return oauthIdentity{accountID: account.AccountID}, nil
	}}}
	target, _ := url.Parse(upstream.URL)
	service.httpClient = &http.Client{Transport: driveRoundTripper(func(r *http.Request) (*http.Response, error) {
		request := r.Clone(r.Context())
		request.URL.Scheme = target.Scheme
		request.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(request)
	})}
	start, err := service.Start(ctx, "fixture", "http://127.0.0.1:5599/callback")
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, _ := url.Parse(start.AuthURL)
	refreshed := make(chan error, 1)
	go func() {
		_, err := source.Token()
		refreshed <- err
	}()
	<-started
	completed := make(chan error, 1)
	go func() {
		completed <- service.Callback(ctx, authorizationURL.Query().Get("state"), "approved-code", "")
	}()
	<-approved
	early := false
	select {
	case err := <-completed:
		early = true
		t.Errorf("approval persisted before in-flight refresh: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if !early {
		if err := <-completed; err != nil {
			t.Fatal(err)
		}
	}
	stored, ok, err := store.LoadToken(ctx, account.ID)
	if err != nil || !ok || stored.AccessToken != "approved" || stored.ClientID != "new-client" || stored.RefreshToken != "approved-refresh" {
		t.Fatalf("old refresh overwrote approved grant: %+v, %v", stored, err)
	}
}

type reconnectOAuthStore struct {
	mu         sync.Mutex
	connection integrations.Connection
	tokens     map[string]integrationoauth.Token
}

func (s *reconnectOAuthStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connection.Provider != provider {
		return nil, nil
	}
	return []integrations.Connection{s.connection}, nil
}

func (s *reconnectOAuthStore) LoadToken(_ context.Context, id string) (integrationoauth.Token, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.tokens[id]
	return token, ok, nil
}

func (s *reconnectOAuthStore) SaveToken(_ context.Context, id string, token integrationoauth.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[id] = token
	return nil
}

func (s *reconnectOAuthStore) SaveOAuthConnection(_ context.Context, token integrationoauth.Token, connection integrations.Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[connection.ID] = token
	s.connection = connection
	return nil
}
