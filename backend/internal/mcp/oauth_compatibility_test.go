package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

func TestOAuthRejectsUnadvertisedS256(t *testing.T) {
	for _, methods := range [][]string{nil, {}, {"plain"}, {"unknown"}} {
		t.Run(fmt.Sprint(methods), func(t *testing.T) {
			srv := mockAuthServer(t, map[string]any{"code_challenge_methods_supported": methods})
			handler := newOAuthHandler(mcpconfig.Server{ID: "pkce", URL: srv.URL + "/mcp"}, newMemTokenStore(), http.DefaultClient)
			handler.mode = oauthModeInteractive
			handler.fetch = func(context.Context, string) (mcpconfig.AuthorizationResponse, error) {
				t.Error("opened authorization without S256 support")
				return mcpconfig.AuthorizationResponse{}, nil
			}
			if err := handler.AuthorizeFromMetadata(t.Context()); err == nil || !strings.Contains(err.Error(), "PKCE") {
				t.Fatalf("authorization error = %v", err)
			}
			if srv.registrations != 0 {
				t.Fatal("registered before checking PKCE")
			}
		})
	}
}

func TestOAuthMissingMetadataDoesNotGuessEndpoints(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	handler := newOAuthHandler(mcpconfig.Server{ID: "missing", URL: srv.URL + "/mcp", OAuth: mcpconfig.OAuthConfig{Issuer: srv.URL}}, newMemTokenStore(), srv.Client())
	handler.mode = oauthModeInteractive
	if err := handler.AuthorizeFromMetadata(t.Context()); err == nil || !strings.Contains(err.Error(), "S256") || posts != 0 {
		t.Fatalf("missing metadata: err=%v posts=%d", err, posts)
	}
}

func TestOAuthRegistrationFailureExplainsSetup(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := mockAuthServer(t, nil)
			srv.registrationStatus = status
			handler := newOAuthHandler(mcpconfig.Server{ID: "protected", URL: srv.URL + "/mcp"}, newMemTokenStore(), srv.Client())
			handler.mode = oauthModeInteractive
			handler.redirectURL = "https://local.example/v1/mcp/oauth/callback"
			handler.broker = oauthbroker.Broker{URL: "https://broker.example/oauth/callback"}
			err := handler.AuthorizeFromMetadata(t.Context())
			for _, want := range []string{"authenticated registration", "https://broker.example/oauth/callback", "configure an OAuth client ID", "S256 PKCE"} {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v; want %q", err, want)
				}
			}
		})
	}
}

func TestOAuthCallbackAndRefreshCompatibility(t *testing.T) {
	for _, test := range []struct {
		name        string
		static      bool
		broker      bool
		cimd        bool
		secret      bool
		oidc        bool
		formToken   bool
		cancelFirst bool
	}{
		{name: "preregistered desktop and browser", static: true, broker: true, cimd: true, oidc: true},
		{name: "preregistered direct callback", static: true},
		{name: "cancelled preregistered retry", static: true, broker: true, cancelFirst: true},
		{name: "preregistered confidential", static: true, broker: true, secret: true},
		{name: "advertised CIMD", broker: true, cimd: true},
		{name: "form encoded token endpoint", static: true, broker: true, formToken: true},
		{name: "supported DCR"},
		{name: "confidential DCR", secret: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var mu sync.Mutex
			var returnURL, state, registeredRedirect, challenge, issuedCode string
			var authorizationRedirect, authorizationClient string
			var registrations, exchanges, refreshes int
			broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/start" {
					var input struct {
						ReturnURL string `json:"return_url"`
					}
					json.NewDecoder(r.Body).Decode(&input)
					mu.Lock()
					returnURL = input.ReturnURL
					state, _ = randomState()
					signedState := state
					mu.Unlock()
					writeJSONResp(w, map[string]any{"state": signedState})
					return
				}
				mu.Lock()
				target, _ := url.Parse(returnURL)
				expected := state
				mu.Unlock()
				if r.URL.Path != "/oauth/callback" || r.URL.Query().Get("state") != expected {
					http.Error(w, "invalid broker state", http.StatusBadRequest)
					return
				}
				target.RawQuery = r.URL.RawQuery
				http.Redirect(w, r, target.String(), http.StatusFound)
			}))
			defer broker.Close()
			mux := http.NewServeMux()
			provider := httptest.NewServer(mux)
			defer provider.Close()
			clientID := "registered-client"
			if !test.static {
				clientID = "dynamic-client"
				if test.cimd {
					clientID = broker.URL + "/oauth/client-metadata.json"
				}
			}
			mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
				writeJSONResp(w, map[string]any{"resource": provider.URL + "/mcp", "authorization_servers": []string{provider.URL}})
			})
			metadataPath := "/.well-known/oauth-authorization-server"
			if test.oidc {
				metadataPath = "/.well-known/openid-configuration"
			}
			mux.HandleFunc(metadataPath, func(w http.ResponseWriter, r *http.Request) {
				writeJSONResp(w, map[string]any{
					"issuer":                                provider.URL,
					"authorization_endpoint":                provider.URL + "/authorize",
					"token_endpoint":                        provider.URL + "/token",
					"registration_endpoint":                 provider.URL + "/register",
					"response_types_supported":              []string{"code"},
					"code_challenge_methods_supported":      []string{"S256"},
					"client_id_metadata_document_supported": test.cimd,
				})
			})
			mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					RedirectURIs []string `json:"redirect_uris"`
				}
				json.NewDecoder(r.Body).Decode(&input)
				mu.Lock()
				registrations++
				registeredRedirect = input.RedirectURIs[0]
				mu.Unlock()
				if test.secret {
					writeJSONResp(w, map[string]any{"client_id": clientID, "client_secret": "registered-secret", "token_endpoint_auth_method": "client_secret_basic"})
					return
				}
				writeJSONResp(w, map[string]any{"client_id": clientID, "token_endpoint_auth_method": "none"})
			})
			mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				mu.Lock()
				defer mu.Unlock()
				if q.Get("client_id") != clientID || q.Get("redirect_uri") != registeredRedirect || q.Get("code_challenge_method") != "S256" || q.Get("resource") != provider.URL+"/mcp" || q.Get("state") == "" {
					t.Errorf("authorization request = %v; callback = %s", q, registeredRedirect)
					http.Error(w, "redirect_uri_mismatch", http.StatusBadRequest)
					return
				}
				authorizationRedirect = q.Get("redirect_uri")
				authorizationClient = q.Get("client_id")
				challenge = q.Get("code_challenge")
				issuedCode, _ = randomState()
				target, _ := url.Parse(authorizationRedirect)
				params := url.Values{"state": {q.Get("state")}, "code": {issuedCode}}
				target.RawQuery = params.Encode()
				http.Redirect(w, r, target.String(), http.StatusFound)
			})
			mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				mu.Lock()
				defer mu.Unlock()
				id := r.Form.Get("client_id")
				if test.secret {
					basicID, secret, ok := r.BasicAuth()
					if !ok || secret != "registered-secret" {
						http.Error(w, "invalid_client", http.StatusUnauthorized)
						return
					}
					id = basicID
				}
				if id != clientID {
					t.Errorf("token client = %q", id)
					http.Error(w, "invalid_client", http.StatusUnauthorized)
					return
				}
				switch r.Form.Get("grant_type") {
				case "authorization_code":
					exchanges++
					if r.Form.Get("redirect_uri") != authorizationRedirect || id != authorizationClient || r.Form.Get("code") != issuedCode || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge || r.Form.Get("resource") != provider.URL+"/mcp" {
						t.Errorf("token request did not match authorization: %v", r.Form)
						http.Error(w, "invalid_grant", http.StatusBadRequest)
						return
					}
				case "refresh_token":
					refreshes++
					if r.Form.Get("refresh_token") != "refresh-token" || r.Form.Get("resource") != provider.URL+"/mcp" {
						t.Errorf("refresh grant = %v", r.Form)
					}
				default:
					t.Errorf("grant = %v", r.Form)
				}
				if test.formToken {
					w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
					io.WriteString(w, "access_token=access-token&refresh_token=refresh-token&token_type=Bearer&expires_in=3600")
					return
				}
				writeJSONResp(w, map[string]any{"access_token": "access-token", "refresh_token": "refresh-token", "token_type": "Bearer", "expires_in": 3600})
			})
			remote := newEchoHandler()
			mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer access-token" {
					w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+provider.URL+`/.well-known/oauth-protected-resource/mcp"`)
					http.Error(w, "sign in required", http.StatusUnauthorized)
					return
				}
				remote.ServeHTTP(w, r)
			})
			server := mcpconfig.Server{ID: "compatibility", URL: provider.URL + "/mcp", Enabled: true}
			if test.static {
				server.OAuth.ClientID = clientID
			}
			if test.secret {
				t.Setenv("MCP_TEST_SECRET", "registered-secret")
				server.OAuth.ClientSecretEnvVar = "MCP_TEST_SECRET"
			}
			store := newMemTokenStore()
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, store, tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			if test.broker {
				manager.broker = oauthbroker.Broker{URL: broker.URL + "/oauth/callback"}
			}
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if err := manager.CompleteAuthorization(r.Context(), mcpconfig.AuthorizationResponse{State: q.Get("state"), Code: q.Get("code"), Error: q.Get("error"), Issuer: q.Get("iss")}); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
			}))
			defer callback.Close()
			localCallback := callback.URL + mcpconfig.OAuthCallbackPath
			mu.Lock()
			if test.static || test.cimd {
				registeredRedirect = manager.OAuthRedirectURL(localCallback)
			}
			mu.Unlock()
			options := mcpconfig.AuthorizeOptions{RedirectURL: localCallback}
			status := manager.Authorize(ctx, server, options)
			if status.AuthURL == "" {
				t.Fatalf("sign-in = %+v", status)
			}
			if duplicate := manager.Authorize(ctx, server, options); !strings.Contains(duplicate.Error, "already in progress") {
				t.Fatalf("overlapping sign-in = %+v", duplicate)
			}
			if err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: "wrong-state", Code: "forged-code", Error: ""}); err == nil {
				t.Fatal("accepted wrong state")
			}
			if test.cancelFirst {
				firstURL, _ := url.Parse(status.AuthURL)
				firstState := firstURL.Query().Get("state")
				err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: firstState, Error: "access_denied"})
				if err == nil || !strings.Contains(err.Error(), "access_denied") {
					t.Fatalf("cancelled consent = %v", err)
				}
				if _, ok, _ := store.LoadToken(ctx, server.TokenID()); ok {
					t.Fatal("cancelled consent persisted a token")
				}
				status = manager.Authorize(ctx, server, options)
				if status.AuthURL == "" {
					t.Fatalf("retry after cancellation = %+v", status)
				}
				if err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: firstState, Code: "late-code"}); err == nil {
					t.Fatal("cancelled callback consumed replacement flow")
				}
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, status.AuthURL, nil)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusOK || manager.Status(server.ID).Status != "connected" {
				t.Fatalf("callback status=%d connection=%+v", res.StatusCode, manager.Status(server.ID))
			}
			authURL, _ := url.Parse(status.AuthURL)
			if err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: authURL.Query().Get("state"), Code: "replayed-code", Error: ""}); err == nil {
				t.Fatal("accepted replayed callback")
			}
			token, _, _ := store.LoadToken(ctx, server.TokenID())
			if token.RedirectURL != registeredRedirect {
				t.Fatalf("saved callback = %q", token.RedirectURL)
			}
			token.Expiry = time.Now().Add(-time.Hour)
			store.SaveToken(ctx, server.TokenID(), token)
			source, err := newOAuthHandler(server, store, provider.Client()).TokenSource(ctx)
			if err != nil {
				t.Fatal(err)
			}
			refreshed, err := source.Token()
			if err != nil || refreshed.AccessToken != "access-token" {
				t.Fatalf("refresh token=%v err=%v", refreshed, err)
			}
			mu.Lock()
			defer mu.Unlock()
			expectedRegistrations := 0
			if !test.static && !test.cimd {
				expectedRegistrations = 1
			}
			if registrations != expectedRegistrations || exchanges != 1 || refreshes != 1 {
				t.Fatalf("registrations=%d exchanges=%d refreshes=%d", registrations, exchanges, refreshes)
			}
		})
	}
}

func TestOAuthResponseIssuerValidation(t *testing.T) {
	for _, test := range []struct {
		name       string
		advertised bool
		issuer     string
		failure    string
		wantError  bool
	}{
		{name: "optional absent"},
		{name: "optional matching", issuer: "matching"},
		{name: "required matching", advertised: true, issuer: "matching"},
		{name: "required missing", advertised: true, wantError: true},
		{name: "optional mismatch", issuer: "https://other.example", wantError: true},
		{name: "required mismatch", advertised: true, issuer: "https://other.example", wantError: true},
		{name: "error mismatch", issuer: "https://other.example", failure: "untrusted-provider-error", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := mockAuthServer(t, map[string]any{"authorization_response_iss_parameter_supported": test.advertised})
			store := newMemTokenStore()
			handler := newOAuthHandler(mcpconfig.Server{ID: "issuer", URL: srv.URL + "/mcp"}, store, srv.Client())
			handler.mode = oauthModeInteractive
			handler.redirectURL = "https://jaz.example/v1/mcp/oauth/callback"
			handler.fetch = func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
				u, _ := url.Parse(authURL)
				issuer := test.issuer
				if issuer == "matching" {
					issuer = srv.URL
				}
				return mcpconfig.AuthorizationResponse{State: u.Query().Get("state"), Code: "approved-code", Issuer: issuer, Error: test.failure}, nil
			}
			err := handler.AuthorizeFromMetadata(t.Context())
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "issuer") || strings.Contains(err.Error(), "untrusted-provider-error") || srv.tokenForm != nil {
					t.Fatalf("issuer rejection: err=%v token=%v", err, srv.tokenForm)
				}
				return
			}
			if err != nil || srv.tokenForm.Get("code") != "approved-code" {
				t.Fatalf("valid issuer: err=%v token=%v", err, srv.tokenForm)
			}
		})
	}
}

func TestOAuthBindsPreregisteredClientToIssuer(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			srv := mockAuthServer(t, nil)
			store := newMemTokenStore()
			server := mcpconfig.Server{ID: "bound", URL: srv.URL + "/mcp", OAuth: mcpconfig.OAuthConfig{ClientID: "registered"}}
			if configured {
				server.OAuth.Issuer = "https://original.example"
			} else {
				store.SaveToken(t.Context(), server.TokenID(), integrationoauth.Token{ClientID: "registered", Issuer: "https://original.example"})
			}
			handler := newOAuthHandler(server, store, srv.Client())
			handler.mode = oauthModeInteractive
			err := handler.AuthorizeFromMetadata(t.Context())
			if err == nil || !strings.Contains(err.Error(), "authorization server") || srv.registrations != 0 || srv.tokenForm != nil {
				t.Fatalf("issuer binding: err=%v registrations=%d token=%v", err, srv.registrations, srv.tokenForm)
			}
		})
	}
}

func TestDCRSelectsApplicationTypeFromCallback(t *testing.T) {
	for _, test := range []struct{ callback, application string }{
		{"http://127.0.0.1:5299/v1/mcp/oauth/callback", "native"},
		{"http://localhost:5299/v1/mcp/oauth/callback", "native"},
		{"http://[::1]:5299/v1/mcp/oauth/callback", "native"},
		{"https://jaz.example/v1/mcp/oauth/callback", "web"},
	} {
		t.Run(test.callback, func(t *testing.T) {
			srv := mockAuthServer(t, nil)
			handler := newOAuthHandler(mcpconfig.Server{ID: "application", URL: srv.URL + "/mcp"}, newMemTokenStore(), srv.Client())
			handler.mode = oauthModeInteractive
			handler.redirectURL = test.callback
			handler.fetch = func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
				u, _ := url.Parse(authURL)
				return mcpconfig.AuthorizationResponse{State: u.Query().Get("state"), Code: "approved-code"}, nil
			}
			if err := handler.AuthorizeFromMetadata(t.Context()); err != nil {
				t.Fatal(err)
			}
			if srv.registration.ApplicationType != test.application || srv.registration.RedirectURIs[0] != test.callback || srv.tokenForm.Get("redirect_uri") != test.callback {
				t.Fatalf("registration=%+v exchange=%v", srv.registration, srv.tokenForm)
			}
		})
	}
}
