package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
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

// mockAuthServer serves the OAuth discovery, registration, and token endpoints an
// MCP server's authorization server would, so we can exercise the full flow.
func mockAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	base := func() string { return srv.URL }

	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"resource":              base() + "/mcp",
			"authorization_servers": []string{base()},
			"scopes_supported":      []string{"read"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"issuer":                           base(),
			"authorization_endpoint":           base() + "/authorize",
			"token_endpoint":                   base() + "/token",
			"registration_endpoint":            base() + "/register",
			"response_types_supported":         []string{"code"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{
			"client_id":                  "dynamic-client-id",
			"token_endpoint_auth_method": "none",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") == "" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code_verifier") == "" {
			http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
			return
		}
		writeJSONResp(w, map[string]any{
			"access_token":  "access-token-123",
			"token_type":    "Bearer",
			"refresh_token": "refresh-token-456",
			"expires_in":    3600,
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
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
	srv := mockAuthServer(t)
	store := newMemTokenStore()

	server := mcpconfig.Server{ID: "srv-oauth", URL: srv.URL + "/mcp"}
	handler := newOAuthHandler(server, store, http.DefaultClient)
	handler.mode = oauthModeInteractive
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	// Fake browser: read the state from the authorization URL and return an
	// authorization code, simulating the user approving access.
	handler.fetch = func(_ context.Context, authURL string) (string, string, error) {
		u, err := url.Parse(authURL)
		if err != nil {
			return "", "", err
		}
		q := u.Query()
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			t.Errorf("authorization URL missing PKCE challenge: %s", authURL)
		}
		return "the-auth-code", q.Get("state"), nil
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

	tok, err := handler.runAuthorization(context.Background(), resp)
	if err != nil {
		t.Fatalf("runAuthorization: %v", err)
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
	t.Setenv("STATIC_OAUTH_SECRET", "static-secret")
	srv := mockStaticAuthServer(t)
	store := newMemTokenStore()

	server := mcpconfig.Server{
		ID:  "srv-static",
		URL: srv.URL + "/mcp",
		OAuth: mcpconfig.OAuthConfig{
			ClientID:           "static-client",
			ClientSecretEnvVar: "STATIC_OAUTH_SECRET",
		},
	}
	handler := newOAuthHandler(server, store, http.DefaultClient)
	handler.mode = oauthModeInteractive
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	handler.fetch = func(_ context.Context, authURL string) (string, string, error) {
		u, err := url.Parse(authURL)
		if err != nil {
			return "", "", err
		}
		q := u.Query()
		if q.Get("client_id") != "static-client" {
			t.Errorf("authorization URL client_id = %q", q.Get("client_id"))
		}
		if q.Get("resource") != srv.URL+"/mcp" {
			t.Errorf("authorization URL resource = %q", q.Get("resource"))
		}
		return "the-auth-code", q.Get("state"), nil
	}

	if err := handler.AuthorizeFromMetadata(context.Background()); err != nil {
		t.Fatalf("AuthorizeFromMetadata: %v", err)
	}
	tok, ok, err := store.LoadToken(context.Background(), mcpconfig.OAuthConnectionID("srv-static"))
	if err != nil || !ok {
		t.Fatalf("LoadToken ok=%v err=%v", ok, err)
	}
	if tok.AccessToken != "static-access" {
		t.Fatalf("token = %#v", tok)
	}
	if tok.ClientID != "" || tok.ClientSecret != "" {
		t.Fatalf("token stored static client fields = %#v", tok)
	}
	if !handler.didAuthorize() {
		t.Fatal("didAuthorize() = false, want true")
	}
}

func TestAuthorizeFromMetadataRequiresStaticClientWhenNoDCR(t *testing.T) {
	srv := mockStaticAuthServer(t)
	server := mcpconfig.Server{ID: "srv-static", URL: srv.URL + "/mcp"}
	handler := newOAuthHandler(server, newMemTokenStore(), http.DefaultClient)
	handler.mode = oauthModeInteractive
	handler.redirectURL = "http://127.0.0.1:5599/callback"
	handler.fetch = func(context.Context, string) (string, string, error) {
		t.Fatal("fetch should not run without a client")
		return "", "", nil
	}

	err := handler.AuthorizeFromMetadata(context.Background())
	if err == nil || !strings.Contains(err.Error(), "configure an OAuth client ID") {
		t.Fatalf("err = %v, want configure client id", err)
	}
}

func TestNonInteractiveAuthorizeReportsNeedsAuth(t *testing.T) {
	store := newMemTokenStore()
	server := mcpconfig.Server{ID: "srv-x", URL: "https://example.com/mcp"}
	handler := newOAuthHandler(server, store, http.DefaultClient)

	resp := &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(""))}
	err := handler.Authorize(context.Background(), &http.Request{}, resp)
	if err != errNeedsAuthorization {
		t.Fatalf("err = %v, want errNeedsAuthorization", err)
	}
	if !handler.needsAuthorization() {
		t.Fatal("needsAuthorization() = false, want true")
	}
}
