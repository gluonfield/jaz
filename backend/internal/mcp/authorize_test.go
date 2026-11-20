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

func TestInteractiveOAuthUsesManagerLifetime(t *testing.T) {
	for _, test := range []struct {
		name     string
		shutdown bool
	}{
		{name: "HTTP requests end before sign-in"},
		{name: "manager shutdown ends sign-in", shutdown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			provider := mockAuthServer(t, nil)
			remote := newEchoHandler()
			var remoteRequests atomic.Int32
			provider.Config.Handler.(*http.ServeMux).HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				remoteRequests.Add(1)
				if r.Header.Get("Authorization") != "Bearer access-token-123" {
					http.Error(w, "sign in required", http.StatusUnauthorized)
					return
				}
				remote.ServeHTTP(w, r)
			})
			server := mcpconfig.Server{ID: "lifetime", URL: provider.URL + "/mcp", Enabled: true}
			store := newMemTokenStore()
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, store, tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			opts := mcpconfig.AuthorizeOptions{RedirectURL: "http://127.0.0.1:5599/callback"}
			request, finishRequest := context.WithCancel(ctx)
			start := manager.Authorize(request, server, opts)
			finishRequest()
			if start.AuthURL == "" {
				t.Fatalf("authorization did not reach the browser: %+v", start)
			}
			manager.authMu.Lock()
			attempt := manager.authFlows[server.TokenID()]
			manager.authMu.Unlock()
			if attempt == nil {
				t.Fatal("sign-in ended with its initiating HTTP request")
			}
			response := mcpconfig.AuthorizationResponse{State: authorizationStateFromURL(start.AuthURL), Code: "approved-code"}
			if test.shutdown {
				manager.Close()
			} else {
				opts.ReplacePending = true
				if retry := manager.Authorize(request, server, opts); retry.Status != "error" || retry.AuthURL != "" {
					t.Fatalf("canceled request started a replacement attempt: %+v", retry)
				}
				if err := manager.CompleteAuthorization(request, response); err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("callback rejected after HTTP disconnect: %v", err)
				}
			}
			select {
			case <-attempt.done:
			case <-ctx.Done():
				t.Fatal("authorization attempt did not finish")
			}
			_, saved, err := store.LoadToken(ctx, server.TokenID())
			if err != nil || saved == test.shutdown {
				t.Fatalf("authorization grant saved = %t, error = %v", saved, err)
			}
			status := manager.Status(server.ID)
			if !test.shutdown {
				if status.Status != "connected" || status.ToolCount != 1 {
					t.Fatalf("sign-in did not complete after browser disconnect: %+v", status)
				}
				return
			}
			if status.Status != "" {
				t.Fatalf("authorization restored status after manager shutdown: %+v", status)
			}
			if err := manager.CompleteAuthorization(ctx, response); err == nil {
				t.Fatal("stale callback accepted after manager shutdown")
			}
			before := remoteRequests.Load()
			if retry := manager.Authorize(ctx, server, opts); retry.Status != "error" || retry.AuthURL != "" || remoteRequests.Load() != before {
				t.Fatalf("manager dispatched authorization after shutdown: %+v", retry)
			}
		})
	}
}

func TestSignInStartsFreshAndKeepsCredentialsUntilApproval(t *testing.T) {
	for _, test := range []struct {
		name         string
		expiry       time.Duration
		cancel       bool
		blocked      bool
		saveFails    bool
		refreshToken string
	}{
		{name: "valid token", expiry: time.Hour, refreshToken: "refresh-token-456"},
		{name: "revoked refresh token", expiry: -time.Hour, refreshToken: "refresh-token-456"},
		{name: "cancelled sign-in", expiry: -time.Hour, cancel: true},
		{name: "reauthorized same refresh token", expiry: -time.Hour, blocked: true, refreshToken: "old-refresh"},
		{name: "reauthorized without refresh token", expiry: -time.Hour, blocked: true},
		{name: "blocked cancelled sign-in", expiry: -time.Hour, blocked: true, cancel: true},
		{name: "blocked failed authorization save", expiry: -time.Hour, blocked: true, saveFails: true, refreshToken: "old-refresh"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			srv := mockAuthServer(t, nil)
			srv.refreshToken = test.refreshToken
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
			var tokens tokenStore = store
			if test.saveFails {
				tokens = refreshSaveFailure{store}
			}
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, tokens, tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			var initialRefreshes int32
			if test.blocked {
				manager.Refresh(ctx)
				initialRefreshes = refreshes.Load()
				if status := manager.Status(server.ID); status.Status != "needs_auth" || initialRefreshes != 1 {
					t.Fatalf("initial blocked grant: status=%+v refreshes=%d", status, initialRefreshes)
				}
			}
			status := manager.runAuthorize(ctx, server, "http://127.0.0.1:5599/callback", func(_ context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
				stored, ok, err := store.LoadToken(ctx, server.TokenID())
				if err != nil || !ok || stored.RefreshToken != old.RefreshToken {
					t.Errorf("credentials changed before approval: %+v, %v", stored, err)
				}
				if test.cancel {
					return mcpconfig.AuthorizationResponse{}, errors.New("consent cancelled")
				}
				u, err := url.Parse(authURL)
				if err != nil {
					return mcpconfig.AuthorizationResponse{}, err
				}
				return mcpconfig.AuthorizationResponse{Code: "approved-code", State: u.Query().Get("state")}, nil
			}, nil)
			if refreshes.Load() != initialRefreshes || staleRequests.Load() != 0 {
				t.Fatalf("sign-in reused saved credentials: refreshes=%d, requests=%d", refreshes.Load(), staleRequests.Load())
			}
			stored, _, err := store.LoadToken(ctx, server.TokenID())
			if err != nil {
				t.Fatal(err)
			}
			if test.cancel || test.saveFails {
				cause := "consent cancelled"
				if test.saveFails {
					cause = "persist token"
				}
				if status.Status != "error" || !strings.Contains(status.Error, cause) || stored.RefreshToken != old.RefreshToken {
					t.Fatalf("failed sign-in: status=%+v token=%+v", status, stored)
				}
				if test.blocked {
					manager.Refresh(ctx)
					if refreshes.Load() != initialRefreshes {
						t.Fatal("failed authorization released the failed grant")
					}
				}
				return
			}
			if status.Status != "connected" || status.ToolCount != 1 || stored.RefreshToken != test.refreshToken {
				t.Fatalf("sign-in did not replace credentials and install tools: status=%+v token=%+v", status, stored)
			}
		})
	}
}

func TestCallbackCancellationAndExpiryLeaveOtherFlowsUsable(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "expired"}[expire], func(t *testing.T) {
			manager := NewManager(&testStore{}, newMemTokenStore(), tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			ctx, cancel := context.WithCancel(t.Context())
			if expire {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
			}
			defer cancel()
			started := make(chan string, 2)
			old := &callbackReceiver{manager: manager, pending: newAuthorizationPending(), started: started}
			other := &callbackReceiver{manager: manager, pending: newAuthorizationPending(), started: started}
			oldDone := make(chan error, 1)
			go func() {
				_, err := old.fetch(ctx, "https://auth.example/authorize?state=old-state")
				oldDone <- err
			}()
			go func() {
				_, err := other.fetch(t.Context(), "https://auth.example/authorize?state=other-state")
				status := mcpconfig.ServerStatus{Status: "connected"}
				if err != nil {
					status = mcpconfig.ServerStatus{Status: "error", Error: err.Error()}
				}
				other.pending.complete(status)
			}()
			for range 2 {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("authorization did not start")
				}
			}
			if expire {
				<-ctx.Done()
			} else {
				cancel()
			}
			if err := <-oldDone; !errors.Is(err, ctx.Err()) {
				t.Fatalf("cancelled fetch = %v", err)
			}
			if err := manager.CompleteAuthorization(t.Context(), mcpconfig.AuthorizationResponse{State: "old-state", Code: "late-code", Error: ""}); err == nil {
				t.Fatal("accepted cancelled or expired state")
			}
			if err := manager.CompleteAuthorization(t.Context(), mcpconfig.AuthorizationResponse{State: "unrelated-state", Code: "code", Error: ""}); err == nil {
				t.Fatal("accepted unknown state")
			}
			if err := manager.CompleteAuthorization(t.Context(), mcpconfig.AuthorizationResponse{State: "other-state", Code: "approved-code", Error: ""}); err != nil {
				t.Fatalf("other authorization was lost: %v", err)
			}
		})
	}
}
