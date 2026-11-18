package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestWarmCatalogRefreshDoesNotReplayFailedTokenRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "warm"}, nil)
		defer func() {
			for session := range remote.Sessions() {
				_ = session.Close()
			}
		}()
		remote.AddTool(&mcpsdk.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{}, nil
		})
		handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true})
		var presentations atomic.Int32
		previous := http.DefaultTransport
		http.DefaultTransport = recoveryTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/token" {
				presentations.Add(1)
				return nil, io.ErrUnexpectedEOF
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			return recorder.Result(), nil
		})
		defer func() {
			http.DefaultTransport = previous
		}()
		server := mcpconfig.Server{ID: "warm-reconciliation", Name: "warm", URL: "http://provider.test/mcp", Enabled: true}
		store := newMemTokenStore()
		if err := store.SaveToken(t.Context(), server.TokenID(), integrationoauth.Token{
			AccessToken: "warm-access", RefreshToken: "warm-refresh", TokenType: "Bearer",
			Expiry: time.Now().Add(11 * time.Second), ClientID: "client",
			TokenURL: "http://provider.test/token", AuthStyle: int(oauth2.AuthStyleInParams),
		}); err != nil {
			t.Fatal(err)
		}
		manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, store, tools.NewRegistry(), log.New(io.Discard))
		defer manager.Close()
		manager.Refresh(t.Context())
		if status := manager.Status(server.ID); status.Status != "connected" {
			t.Fatalf("initial connection failed: %+v", status)
		}
		time.Sleep(2 * time.Second)
		manager.Refresh(t.Context())
		if got := presentations.Load(); got != 1 {
			t.Fatalf("warm catalog reconciliation presented the refresh token %d times, want 1", got)
		}
		if status := manager.Status(server.ID); status.Status != "error" || status.Retryable {
			t.Fatalf("ambiguous refresh failure status = %+v", status)
		}
		otherStore := newMemTokenStore()
		grant, _, _ := store.LoadToken(t.Context(), server.TokenID())
		if err := otherStore.SaveToken(t.Context(), server.TokenID(), grant); err != nil {
			t.Fatal(err)
		}
		other := NewManager(&testStore{servers: []mcpconfig.Server{server}}, otherStore, tools.NewRegistry(), log.New(io.Discard))
		defer other.Close()
		other.Refresh(t.Context())
		manager.Refresh(t.Context())
		if got := presentations.Load(); got != 2 {
			t.Fatalf("independent managers presented the grant %d times, want 1 each", got)
		}
	})
}
