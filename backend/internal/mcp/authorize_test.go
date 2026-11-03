package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

func TestSignInStartsFreshAndKeepsCredentialsUntilApproval(t *testing.T) {
	for _, test := range []struct {
		name   string
		expiry time.Duration
		cancel bool
	}{
		{name: "valid token", expiry: time.Hour},
		{name: "revoked refresh token", expiry: -time.Hour},
		{name: "cancelled sign-in", expiry: -time.Hour, cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			srv := mockAuthServer(t, nil)
			mux := srv.Config.Handler.(*http.ServeMux)
			var refreshes, staleRequests atomic.Int32
			mux.HandleFunc("/expired-token", func(w http.ResponseWriter, _ *http.Request) {
				refreshes.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				writeJSONResp(w, map[string]any{"error": "invalid_grant"})
			})
			remote := newEchoHandler()
			mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "Bearer old-access" {
					staleRequests.Add(1)
				}
				if r.Header.Get("Authorization") != "Bearer access-token-123" {
					w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource/mcp"`)
					http.Error(w, "sign in required", http.StatusUnauthorized)
					return
				}
				remote.ServeHTTP(w, r)
			})
			server := mcpconfig.Server{ID: "crm", URL: srv.URL + "/mcp", Enabled: true}
			store := newMemTokenStore()
			old := integrationoauth.Token{
				AccessToken: "old-access", RefreshToken: "old-refresh", TokenType: "Bearer",
				Expiry: time.Now().Add(test.expiry), TokenURL: srv.URL + "/expired-token",
				ClientID: "old-client", AuthStyle: int(oauth2.AuthStyleInParams),
			}
			if err := store.SaveToken(ctx, server.TokenID(), old); err != nil {
				t.Fatal(err)
			}
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, store, tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			status := manager.runAuthorize(ctx, server, "http://127.0.0.1:5599/callback", func(_ context.Context, authURL string) (string, string, error) {
				stored, ok, err := store.LoadToken(ctx, server.TokenID())
				if err != nil || !ok || stored.RefreshToken != old.RefreshToken {
					t.Errorf("credentials changed before approval: %+v, %v", stored, err)
				}
				if test.cancel {
					return "", "", errors.New("consent cancelled")
				}
				u, err := url.Parse(authURL)
				if err != nil {
					return "", "", err
				}
				return "approved-code", u.Query().Get("state"), nil
			}, nil)
			if refreshes.Load() != 0 || staleRequests.Load() != 0 {
				t.Fatalf("sign-in reused saved credentials: refreshes=%d, requests=%d", refreshes.Load(), staleRequests.Load())
			}
			stored, _, err := store.LoadToken(ctx, server.TokenID())
			if err != nil {
				t.Fatal(err)
			}
			if test.cancel {
				if status.Status != "error" || !strings.Contains(status.Error, "consent cancelled") || stored.RefreshToken != old.RefreshToken {
					t.Fatalf("cancelled sign-in: status=%+v token=%+v", status, stored)
				}
				return
			}
			if status.Status != "connected" || status.ToolCount != 1 || stored.RefreshToken != "refresh-token-456" {
				t.Fatalf("sign-in did not replace credentials and install tools: status=%+v token=%+v", status, stored)
			}
		})
	}
}
