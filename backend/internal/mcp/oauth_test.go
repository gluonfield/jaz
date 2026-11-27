package mcp

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type memTokenStore struct {
	mu     sync.Mutex
	tokens map[string]integrationoauth.Token
}

func newMemTokenStore() *memTokenStore {
	return &memTokenStore{tokens: map[string]integrationoauth.Token{}}
}

func (m *memTokenStore) LoadToken(_ context.Context, id string) (integrationoauth.Token, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	return t, ok, nil
}

func (m *memTokenStore) SaveToken(_ context.Context, id string, t integrationoauth.Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[id] = t
	return nil
}

func (m *memTokenStore) DeleteToken(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, id)
	return nil
}

type mockAuthorizationServer struct {
	*httptest.Server
	registrations      int
	registrationStatus int
	registration       oauthex.ClientRegistrationMetadata
	tokenForm          url.Values
	refreshToken       string
	tokenExpiresIn     int
}

// mockAuthServer serves the OAuth discovery, registration, and token endpoints an
// MCP server's authorization server would, so we can exercise the full flow.
// extra adds authorization server metadata.
func mockAuthServer(t *testing.T, extra map[string]any) *mockAuthorizationServer {
	t.Helper()
	mux := http.NewServeMux()
	srv := &mockAuthorizationServer{Server: httptest.NewServer(mux), refreshToken: "refresh-token-456", tokenExpiresIn: 3600}
	t.Cleanup(srv.Close)
	base := srv.URL

	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"resource":              base + "/mcp",
			"authorization_servers": []string{base},
			"scopes_supported":      []string{"read"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		metadata := map[string]any{
			"issuer":                           base,
			"authorization_endpoint":           base + "/authorize",
			"token_endpoint":                   base + "/token",
			"registration_endpoint":            base + "/register",
			"response_types_supported":         []string{"code"},
			"code_challenge_methods_supported": []string{"S256"},
		}
		maps.Copy(metadata, extra)
		writeJSONResp(w, metadata)
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		srv.registrations++
		json.NewDecoder(r.Body).Decode(&srv.registration)
		if srv.registrationStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(srv.registrationStatus)
			io.WriteString(w, `{"error":"invalid_client","error_description":"initial credentials required"}`)
			return
		}
		writeJSONResp(w, map[string]any{
			"client_id":                  "dynamic-client-id",
			"token_endpoint_auth_method": "none",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		srv.tokenForm = r.Form
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") == "" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code_verifier") == "" {
			http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
			return
		}
		response := map[string]any{"access_token": "access-token-123", "token_type": "Bearer"}
		if srv.tokenExpiresIn > 0 {
			response["expires_in"] = srv.tokenExpiresIn
		}
		if srv.refreshToken != "" {
			response["refresh_token"] = srv.refreshToken
		}
		writeJSONResp(w, response)
	})
	return srv
}

func mockStaticAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	base := func() string { return srv.URL }

	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"resource":              base() + "/mcp",
			"authorization_servers": []string{base()},
			"scopes_supported":      []string{"read", "write"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"issuer":                           base(),
			"authorization_endpoint":           base() + "/authorize",
			"token_endpoint":                   base() + "/token",
			"response_types_supported":         []string{"code"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		clientID, clientSecret, ok := r.BasicAuth()
		if !ok {
			clientID = r.Form.Get("client_id")
			clientSecret = r.Form.Get("client_secret")
		}
		if clientID != "static-client" || clientSecret != "static-secret" {
			http.Error(w, "bad client credentials", http.StatusUnauthorized)
			return
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") == "" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code_verifier") == "" {
			http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
			return
		}
		writeJSONResp(w, map[string]any{
			"access_token":  "static-access",
			"token_type":    "Bearer",
			"refresh_token": "static-refresh",
			"expires_in":    3600,
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSONResp(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func TestRunAuthorizationCompletesFlow(t *testing.T) {
	srv := mockAuthServer(t, nil)
	store := newMemTokenStore()

	server := mcpconfig.Server{ID: "srv-oauth", URL: srv.URL + "/mcp"}
	handler := newOAuthHandler(server, store, http.DefaultClient, nil)
	handler.interactive = true
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	// Fake browser: read the state from the authorization URL and return an
	// authorization code, simulating the user approving access.
	handler.fetch = func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
		u, err := url.Parse(authURL)
		if err != nil {
			return mcpconfig.AuthorizationResponse{}, err
		}
		q := u.Query()
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			t.Errorf("authorization URL missing PKCE challenge: %s", authURL)
		}
		return mcpconfig.AuthorizationResponse{Code: "the-auth-code", State: q.Get("state")}, nil
	}

	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"Www-Authenticate": []string{
				`Bearer resource_metadata="` + srv.URL + `/.well-known/oauth-protected-resource/mcp"`,
			},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}

	if err := handler.Authorize(t.Context(), nil, resp); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	tok, ok, err := store.LoadToken(t.Context(), server.TokenID())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("authorization did not save the token")
	}
	if tok.AccessToken != "access-token-123" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
	if tok.RefreshToken != "refresh-token-456" {
		t.Errorf("refresh token = %q", tok.RefreshToken)
	}
	if tok.ClientID != "dynamic-client-id" {
		t.Errorf("client id = %q", tok.ClientID)
	}
	if tok.TokenURL != srv.URL+"/token" {
		t.Errorf("token url = %q", tok.TokenURL)
	}
}

func TestAuthorizeFromMetadataUsesStaticOAuthClient(t *testing.T) {
	for _, test := range []struct {
		name      string
		appScopes bool
		challenge string
		scope     string
	}{
		{name: "discovered-scopes", scope: "read write"},
		{name: "app-defaults", appScopes: true},
		{name: "challenge-overrides-app-defaults", appScopes: true, challenge: `Bearer scope="read"`, scope: "read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("STATIC_OAUTH_SECRET", "static-secret")
			srv := mockStaticAuthServer(t)
			store := newMemTokenStore()

			server := mcpconfig.Server{
				ID:  "srv-static",
				URL: srv.URL + "/mcp",
				OAuth: mcpconfig.OAuthConfig{
					ClientID:           "static-client",
					UseAppScopes:       test.appScopes,
					ClientSecretEnvVar: "STATIC_OAUTH_SECRET",
				},
			}
			handler := newOAuthHandler(server, store, http.DefaultClient, nil)
			handler.interactive = true
			handler.redirectURL = "http://127.0.0.1:5599/callback"
			handler.fetch = func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
				u, err := url.Parse(authURL)
				if err != nil {
					return mcpconfig.AuthorizationResponse{}, err
				}
				q := u.Query()
				if q.Get("scope") != test.scope || test.scope == "" && q.Has("scope") {
					t.Errorf("scope parameter = %v, want %q (omitted when empty)", q["scope"], test.scope)
				}
				if q.Get("client_id") != "static-client" {
					t.Errorf("authorization URL client_id = %q", q.Get("client_id"))
				}
				if q.Get("resource") != srv.URL+"/mcp" {
					t.Errorf("authorization URL resource = %q", q.Get("resource"))
				}
				return mcpconfig.AuthorizationResponse{Code: "the-auth-code", State: q.Get("state")}, nil
			}

			var err error
			if test.challenge == "" {
				err = handler.authorize(context.Background(), nil)
			} else {
				err = handler.Authorize(context.Background(), nil, &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Www-Authenticate": {test.challenge}}, Body: io.NopCloser(strings.NewReader(""))})
			}
			if err != nil {
				t.Fatalf("authorize: %v", err)
			}
			tok, ok, err := store.LoadToken(context.Background(), mcpconfig.OAuthConnectionID("srv-static"))
			if err != nil || !ok {
				t.Fatalf("LoadToken ok=%v err=%v", ok, err)
			}
			if tok.AccessToken != "static-access" {
				t.Fatalf("token = %#v", tok)
			}
			if tok.ClientID != "static-client" || tok.Issuer != srv.URL || tok.ClientSecret != "" {
				t.Fatalf("token stored static client fields = %#v", tok)
			}
			if !handler.didAuthorize() {
				t.Fatal("didAuthorize() = false, want true")
			}

		})
	}
}

func TestAuthorizeFromMetadataRequiresStaticClientWhenNoDCR(t *testing.T) {
	srv := mockStaticAuthServer(t)
	server := mcpconfig.Server{ID: "srv-static", URL: srv.URL + "/mcp"}
	handler := newOAuthHandler(server, newMemTokenStore(), http.DefaultClient, nil)
	handler.interactive = true
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	handler.fetch = func(context.Context, string) (mcpconfig.AuthorizationResponse, error) {
		t.Fatal("fetch should not run without a client")
		return mcpconfig.AuthorizationResponse{}, nil
	}

	err := handler.authorize(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "configure an OAuth client ID") {
		t.Fatalf("err = %v, want configure client id", err)
	}
}

func TestAuthorizeFromMetadataPrefersClientMetadataDocument(t *testing.T) {
	var returnURLs []string
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ReturnURL string `json:"return_url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		returnURLs = append(returnURLs, req.ReturnURL)
		writeJSONResp(w, map[string]any{"state": "broker-state"})
	}))
	t.Cleanup(broker.Close)
	brokerURL := broker.URL + "/oauth/callback"
	clientID := broker.URL + "/oauth/client-metadata.json"
	srv := mockAuthServer(t, map[string]any{"client_id_metadata_document_supported": true})

	store := newMemTokenStore()
	handler := newOAuthHandler(mcpconfig.Server{ID: "srv-cimd", URL: srv.URL + "/mcp"}, store, http.DefaultClient, nil)
	handler.interactive = true
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	handler.broker = oauthbroker.Broker{URL: brokerURL}
	handler.fetch = func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
		u, _ := url.Parse(authURL)
		q := u.Query()
		if q.Get("client_id") != clientID || q.Get("redirect_uri") != brokerURL || q.Get("state") != "broker-state" {
			t.Errorf("authorization URL = %s", authURL)
		}
		return mcpconfig.AuthorizationResponse{Code: "the-auth-code", State: q.Get("state")}, nil
	}

	if err := handler.authorize(context.Background(), nil); err != nil {
		t.Fatalf("AuthorizeFromMetadata: %v", err)
	}
	if srv.registrations != 0 || srv.tokenForm.Get("client_id") != clientID || srv.tokenForm.Get("redirect_uri") != brokerURL {
		t.Fatalf("registrations = %d, token request = %v", srv.registrations, srv.tokenForm)
	}
	if !slices.Equal(returnURLs, []string{handler.redirectURL}) {
		t.Fatalf("broker return URLs = %#v, want the Jaz callback", returnURLs)
	}
	tok, _, _ := store.LoadToken(context.Background(), mcpconfig.OAuthConnectionID("srv-cimd"))
	if tok.ClientID != clientID {
		t.Fatalf("stored client ID = %q, want the metadata document", tok.ClientID)
	}
}

func TestNonInteractiveAuthorizeReportsNeedsAuth(t *testing.T) {
	store := newMemTokenStore()
	server := mcpconfig.Server{ID: "srv-x", URL: "https://example.com/mcp"}
	handler := newOAuthHandler(server, store, http.DefaultClient, nil)

	resp := &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(""))}
	err := handler.Authorize(context.Background(), &http.Request{}, resp)
	if err != errNeedsAuthorization {
		t.Fatalf("err = %v, want errNeedsAuthorization", err)
	}
	if handler.authorizationChallenge() == nil {
		t.Fatal("authorization challenge was not recorded")
	}
}
