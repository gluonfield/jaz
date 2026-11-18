package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type recoveryTransport func(*http.Request) (*http.Response, error)

func (f recoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type refreshSaveFailure struct {
	*memTokenStore
}

func (s refreshSaveFailure) SaveToken(context.Context, string, integrationoauth.Token) error {
	return errors.New("cannot persist rotated token")
}

func TestRecoveryDoesNotRetryFailedOAuthRefresh(t *testing.T) {
	for _, mode := range []string{"response_lost", "grant_rejected", "save_lost"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var requests atomic.Int32
				previous := http.DefaultTransport
				http.DefaultTransport = recoveryTransport(func(req *http.Request) (*http.Response, error) {
					if req.URL.Path != "/token" {
						t.Errorf("unexpected provider request: %s", req.URL.Path)
					}
					requests.Add(1)
					if mode == "response_lost" {
						return nil, io.ErrUnexpectedEOF
					}
					if mode == "save_lost" {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": {"application/json"}},
							Body:       io.NopCloser(strings.NewReader(`{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Header:     http.Header{"Content-Type": {"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`)),
					}, nil
				})
				defer func() {
					http.DefaultTransport = previous
				}()
				server := mcpconfig.Server{ID: "oauth", Name: "OAuth", URL: "http://provider.test/mcp", Enabled: true}
				tokens := newMemTokenStore()
				tokenID := server.TokenID()
				grant := integrationoauth.Token{
					AccessToken: "expired", RefreshToken: "rotating", TokenType: "Bearer",
					Expiry: time.Now().Add(-time.Hour), ClientID: "client",
					TokenURL: "http://provider.test/token", AuthStyle: int(oauth2.AuthStyleInParams),
				}
				if err := tokens.SaveToken(t.Context(), tokenID, grant); err != nil {
					t.Fatal(err)
				}
				var store tokenStore = tokens
				if mode == "save_lost" {
					store = refreshSaveFailure{tokens}
				}
				inventory := &recoveryInventory{servers: []mcpconfig.Server{server}}
				manager := NewManager(inventory, store, tools.NewRegistry(), log.New(io.Discard))
				defer manager.Close()
				go manager.Run()
				synctest.Wait()
				status := manager.Status(server.ID)
				want := "error"
				if mode == "grant_rejected" {
					want = "needs_auth"
				}
				if status.Status != want || status.Retryable {
					t.Fatalf("refresh failure status = %+v", status)
				}
				inventory.replace([]mcpconfig.Server{server, {
					ID: "new", Name: "new", Enabled: true,
					LocalServer: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "new"}, nil),
				}})
				for range 3 {
					req := httptest.NewRequest(http.MethodPost, "http://jaz.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Accept", "application/json, text/event-stream")
					manager.Handler().ServeHTTP(httptest.NewRecorder(), req)
				}
				if manager.Status("new").Status != "connected" {
					t.Fatal("proxy did not discover the new provider")
				}
				time.Sleep(3 * time.Minute)
				synctest.Wait()
				if requests.Load() != 1 {
					t.Fatalf("refresh token was presented %d times, want 1", requests.Load())
				}
				stored, ok, err := tokens.LoadToken(t.Context(), tokenID)
				if err != nil || !ok || stored.RefreshToken != grant.RefreshToken {
					t.Fatal("recovery changed the stored grant")
				}
			})
		})
	}
}
