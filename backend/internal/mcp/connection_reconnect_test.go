package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/server"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type reconnectInventory struct {
	*sqlitestore.Store
	server    mcpconfig.Server
	unrelated mcpconfig.Server
	fail      *atomic.Bool
}

func (s reconnectInventory) ListMCPServers() ([]mcpconfig.Server, error) {
	servers := []mcpconfig.Server{s.unrelated}
	if _, ok, err := s.LoadConnection(context.Background(), s.server.ID); err != nil {
		return nil, err
	} else if ok {
		servers = append(servers, s.server)
	}
	return servers, nil
}

func (s reconnectInventory) SaveConnection(ctx context.Context, connection integrations.Connection) error {
	if s.fail != nil {
		s.fail.Store(true)
	}
	return s.Store.SaveConnection(ctx, connection)
}

func TestConnectionsSignInReconcilesActualTargetTools(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "restores-tools", true: "target-failure"}[unavailable], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			auth := mockAuthServer(t, nil)
			mux := auth.Config.Handler.(*http.ServeMux)
			var refreshes atomic.Int32
			mux.HandleFunc("/old-token", func(w http.ResponseWriter, r *http.Request) {
				refreshes.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
			})
			var fail atomic.Bool
			remote := newEchoHandler()
			mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				if fail.Load() {
					http.Error(w, "target unavailable", http.StatusServiceUnavailable)
					return
				}
				if r.Header.Get("Authorization") != "Bearer access-token-123" {
					w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+auth.URL+`/.well-known/oauth-protected-resource/mcp"`)
					http.Error(w, "sign in required", http.StatusUnauthorized)
					return
				}
				remote.ServeHTTP(w, r)
			})
			var unrelatedRequests atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				unrelatedRequests.Add(1)
				http.Error(w, "unrelated unavailable", http.StatusServiceUnavailable)
			}))
			defer other.Close()
			plugin := integrations.Plugin{ID: "fixture", Name: "Fixture", Provider: integrations.Provider{ID: "fixture", Name: "Fixture"}, Auth: []integrations.AuthOption{{Kind: integrations.AuthKindMCPConnection}}, RemoteMCP: &integrations.RemoteMCP{URL: auth.URL + "/mcp"}}
			id, err := integrations.ConnectionID(plugin.ID, plugin.RemoteMCP.URL)
			if err != nil {
				t.Fatal(err)
			}
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			inventory := reconnectInventory{Store: store, server: mcpconfig.Server{ID: id, Name: plugin.Name, URL: plugin.RemoteMCP.URL, Enabled: true}, unrelated: mcpconfig.Server{ID: "unrelated", URL: other.URL, Enabled: true}}
			if err := store.SaveConnection(ctx, integrations.Connection{ID: id, Provider: plugin.ID, AccountID: plugin.RemoteMCP.URL}); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveToken(ctx, inventory.server.TokenID(), integrationoauth.Token{AccessToken: "expired", RefreshToken: "old-refresh", ClientID: "old-client", TokenURL: auth.URL + "/old-token", AuthStyle: int(oauth2.AuthStyleInParams), Expiry: time.Now().Add(-time.Hour)}); err != nil {
				t.Fatal(err)
			}
			registry := tools.NewRegistry()
			manager := NewManager(inventory, store, registry, log.New(io.Discard))
			defer manager.Close()
			manager.Refresh(ctx)
			if refreshes.Load() == 0 || manager.Status(id).Status == "connected" || len(registry.Definitions()) != 0 {
				t.Fatal("fixture did not reproduce an unusable saved grant")
			}
			before := unrelatedRequests.Load()
			if unavailable {
				inventory.fail = &fail
			}
			start, err := connections.NewMCPConnectionConnector(inventory, manager).Connect(ctx, plugin, "http://127.0.0.1:5599/callback")
			if err != nil {
				t.Fatal(err)
			}
			authorizationURL, err := url.Parse(start.AuthURL)
			if err != nil {
				t.Fatal(err)
			}
			if authorizationURL.Query().Get("client_id") != "dynamic-client-id" {
				t.Fatal("sign-in retained the old OAuth client")
			}
			callback := httptest.NewRecorder()
			query := url.Values{"state": {authorizationURL.Query().Get("state")}, "code": {"approved-code"}}
			(&server.Server{MCP: manager}).Handler().ServeHTTP(callback, httptest.NewRequest(http.MethodGet, mcpconfig.OAuthCallbackPath+"?"+query.Encode(), nil))
			if unrelatedRequests.Load() != before {
				t.Fatal("sign-in reconciled an unrelated server")
			}
			grant, ok, loadErr := store.LoadToken(ctx, inventory.server.TokenID())
			if loadErr != nil || !ok || grant.ClientID != "dynamic-client-id" || grant.RefreshToken != "refresh-token-456" {
				t.Fatalf("reconnect did not persist new grant/client: %+v, %v", grant, loadErr)
			}
			if unavailable {
				if callback.Code != http.StatusBadRequest || !strings.Contains(callback.Body.String(), "Service Unavailable") || len(registry.Definitions()) != 0 {
					t.Fatalf("failed target reported successful sign-in: %d %s", callback.Code, callback.Body)
				}
				return
			}
			if callback.Code != http.StatusOK {
				t.Fatalf("callback = %d %s", callback.Code, callback.Body)
			}
			if status := manager.Status(id); status.Status != "connected" || status.ToolCount != 1 {
				t.Fatalf("sign-in did not restore target tools: %+v", status)
			}
			definitions := registry.Definitions()
			if len(definitions) != 1 {
				t.Fatalf("agent tool count = %d", len(definitions))
			}
			tool, ok := registry.Get(tools.DefinitionName(definitions[0]))
			if !ok {
				t.Fatal("restored tool missing from agent registry")
			}
			if _, err := tool.Execute(ctx, map[string]any{"value": "reconnected"}); err != nil {
				t.Fatalf("restored tool is unusable: %v", err)
			}
		})
	}
}
