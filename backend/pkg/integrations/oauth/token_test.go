package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type memoryStore struct {
	token Token
	ok    bool
}

func (m *memoryStore) LoadToken(context.Context, string) (Token, bool, error) {
	return m.token, m.ok, nil
}

func (m *memoryStore) SaveToken(_ context.Context, _ string, token Token) error {
	m.token = token
	m.ok = true
	return nil
}

func TestRefresherPersistsUpdatedToken(t *testing.T) {
	tokenEndpointCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenEndpointCalled = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("unexpected refresh request %s", r.Form.Encode())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	store := &memoryStore{
		ok: true,
		token: Token{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(-time.Hour),
			ClientID:     "client",
			TokenURL:     server.URL,
			AuthStyle:    int(oauth2.AuthStyleInParams),
			Resource:     "https://mcp.example.com",
		},
	}
	got, err := (Refresher{Store: store}).FreshToken(context.Background(), "conn")
	if err != nil {
		t.Fatal(err)
	}
	if !tokenEndpointCalled {
		t.Fatal("expected token endpoint call")
	}
	if got.AccessToken != "new-access" || store.token.AccessToken != "new-access" || store.token.RefreshToken != "new-refresh" {
		t.Fatalf("token = %#v stored = %#v", got, store.token)
	}
	if got.ClientID != "client" || got.TokenURL != server.URL || got.Resource != "https://mcp.example.com" {
		t.Fatalf("fresh token metadata = %#v", got)
	}
}

func TestRefresherUsesResolvedClientConfig(t *testing.T) {
	tokenEndpointCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenEndpointCalled = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "current-client" || r.Form.Get("client_secret") != "current-secret" {
			t.Fatalf("refresh used stale client config: %s", r.Form.Encode())
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	store := &memoryStore{
		ok: true,
		token: Token{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(-time.Hour),
			ClientID:     "stale-client",
			TokenURL:     server.URL,
			AuthStyle:    int(oauth2.AuthStyleInParams),
		},
	}
	_, err := (Refresher{
		Store: store,
		ClientConfig: func(_ context.Context, token Token) (ClientConfig, error) {
			return ClientConfig{
				ClientID:     "current-client",
				ClientSecret: "current-secret",
				TokenURL:     token.TokenURL,
				AuthStyle:    oauth2.AuthStyleInParams,
			}, nil
		},
	}).FreshToken(context.Background(), "conn")
	if err != nil {
		t.Fatal(err)
	}
	if !tokenEndpointCalled {
		t.Fatal("expected token endpoint call")
	}
}
