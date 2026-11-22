package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

type reconnectInventory struct {
	*sqlitestore.Store
	plugins    *plugins.Store
	unrelated  mcpconfig.Server
	beforeSave func(context.Context)
}

func (s *reconnectInventory) ListMCPServers() ([]mcpconfig.Server, error) {
	servers := []mcpconfig.Server{s.unrelated}
	installed, err := s.plugins.ListMCPServers()
	if err != nil {
		return nil, err
	}
	return append(servers, installed...), nil
}

func (s *reconnectInventory) SaveConnection(ctx context.Context, connection integrations.Connection) error {
	if s.beforeSave != nil {
		s.beforeSave(ctx)
	}
	return s.Store.SaveConnection(ctx, connection)
}

func writeRemoteMCPPlugin(t *testing.T, layout runtimefiles.Layout, pluginID, displayName, endpointURL string) {
	t.Helper()
	dir := filepath.Join(layout.Plugins, pluginID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(map[string]any{
		"name":        pluginID,
		"displayName": displayName,
	})
	if err != nil {
		t.Fatal(err)
	}
	mcpJSON, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			pluginID: map[string]any{
				"type": "http",
				"url":  endpointURL,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), mcpJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionsSignInReconcilesActualTargetTools(t *testing.T) {
	for _, test := range []struct {
		name        string
		warmed      bool
		unavailable bool
		retry       bool
	}{
		{name: "restores-tools"},
		{name: "warmed-existing-client", warmed: true},
		{name: "target-failure", unavailable: true},
		{name: "replaces-abandoned-browser", retry: true},
	} {
		t.Run(test.name, func(t *testing.T) {
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
			var allowOld atomic.Bool
			var lastAuthorization atomic.Value
			remote := newEchoHandler()
			mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				if fail.Load() {
					http.Error(w, "target unavailable", http.StatusServiceUnavailable)
					return
				}
				authorization := r.Header.Get("Authorization")
				lastAuthorization.Store(authorization)
				if authorization != "Bearer access-token-123" && !(allowOld.Load() && authorization == "Bearer unexpired-old") {
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

			root := t.TempDir()
			layout := runtimefiles.New(root)
			endpointURL := auth.URL + "/mcp"
			writeRemoteMCPPlugin(t, layout, "fixture", "Fixture", endpointURL)

			store, err := sqlitestore.New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()

			catalog := connections.NewCatalog(nil)
			pluginStore := plugins.NewStore(layout, store, catalog)
			id := plugins.MCPServerID("fixture", "fixture")
			targetServer := mcpconfig.Server{ID: id, Name: "fixture:fixture", Transport: mcpconfig.TransportStreamableHTTP, URL: endpointURL, Enabled: true}
			inventory := &reconnectInventory{
				Store:     store,
				plugins:   pluginStore,
				unrelated: mcpconfig.Server{ID: "unrelated", URL: other.URL, Enabled: true},
			}
			saving := make(chan struct{})
			saveAllowed := make(chan struct{})
			if test.retry {
				inventory.beforeSave = func(ctx context.Context) {
					close(saving)
					select {
					case <-saveAllowed:
					case <-ctx.Done():
					}
				}
			}
			if err := store.SaveConnection(ctx, integrations.Connection{ID: id, Provider: "fixture", AccountID: endpointURL, AccountName: "Fixture"}); err != nil {
				t.Fatal(err)
			}
			old := integrationoauth.Token{AccessToken: "expired", RefreshToken: "old-refresh", ClientID: "old-client", TokenURL: auth.URL + "/old-token", AuthStyle: int(oauth2.AuthStyleInParams), Expiry: time.Now().Add(-time.Hour)}
			if test.warmed {
				old.AccessToken = "unexpired-old"
				old.Expiry = time.Now().Add(time.Hour)
			}
			if err := store.SaveToken(ctx, targetServer.TokenID(), old); err != nil {
				t.Fatal(err)
			}
			registry := tools.NewRegistry()
			manager := NewManager(inventory, store, registry, log.New(io.Discard))
			defer manager.Close()

			connService := connections.NewService(catalog, store)
			connector := connections.NewMCPConnectionConnector(inventory, manager)
			connectService := connections.NewConnectService(catalog, nil, nil, connector)
			pluginService := plugins.NewService(pluginStore, store, connService, connectService, nil, nil)

			var existing *mcpsdk.ClientSession
			if test.warmed {
				allowOld.Store(true)
				client, err := (integrationoauth.Refresher{Store: store}).Client(ctx, targetServer.TokenID())
				if err != nil {
					t.Fatal(err)
				}
				existing, err = mcpsdk.NewClient(&mcpsdk.Implementation{Name: "existing-client"}, nil).Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: targetServer.URL, HTTPClient: client, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer existing.Close()
				result, err := existing.CallTool(ctx, &mcpsdk.CallToolParams{Name: "echo", Arguments: map[string]any{"value": "before sign-in"}})
				if err != nil || result.IsError || lastAuthorization.Load() != "Bearer unexpired-old" {
					t.Fatalf("existing client did not use the unexpired old grant: %+v, %v", result, err)
				}
				allowOld.Store(false)
			}
			manager.Refresh(ctx)
			if (!test.warmed && refreshes.Load() == 0) || manager.Status(id).Status == "connected" || len(registry.Definitions()) != 0 {
				t.Fatal("fixture did not reproduce an unusable saved grant")
			}
			before := unrelatedRequests.Load()
			if test.unavailable {
				inventory.beforeSave = func(context.Context) { fail.Store(true) }
			}
			opts := mcpconfig.AuthorizeOptions{RedirectURL: "http://127.0.0.1:5599/callback"}
			startConnect := func() (connections.ConnectStart, error) {
				result, err := pluginService.Connect(ctx, "fixture", connections.StartOptions{MCP: opts})
				return result.Start, err
			}
			start, err := startConnect()
			if err != nil {
				t.Fatal(err)
			}
			handler := (&server.Server{MCP: manager}).Handler()
			respond := func(authURL, code string) *httptest.ResponseRecorder {
				callback := httptest.NewRecorder()
				query := url.Values{"state": {authorizationStateFromURL(authURL)}, "code": {code}}
				handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, mcpconfig.OAuthCallbackPath+"?"+query.Encode(), nil).WithContext(ctx))
				return callback
			}
			if test.retry {
				manager.authMu.Lock()
				oldAttempt := manager.authFlows[targetServer.TokenID()]
				manager.authMu.Unlock()
				replacement, err := startConnect()
				if err != nil || authorizationStateFromURL(start.AuthURL) == authorizationStateFromURL(replacement.AuthURL) {
					t.Fatalf("retry did not start a fresh sign-in: %v", err)
				}
				select {
				case <-oldAttempt.done:
				case <-ctx.Done():
					t.Fatal("abandoned OAuth attempt did not finish")
				}
				if conflict := manager.Authorize(ctx, targetServer, opts); !strings.Contains(conflict.Error, "already in progress") {
					t.Fatalf("old attempt cleanup cleared replacement: %+v", conflict)
				}
				if late := respond(start.AuthURL, "late-code"); late.Code != http.StatusBadRequest {
					t.Fatalf("abandoned OAuth callback accepted: status = %d", late.Code)
				}
				if grant, ok, err := store.LoadToken(ctx, targetServer.TokenID()); err != nil || !ok || grant.AccessToken != old.AccessToken {
					t.Fatal("abandoned callback changed the previous grant")
				}
				start = replacement
			}
			authorizationURL, err := url.Parse(start.AuthURL)
			if err != nil {
				t.Fatal(err)
			}
			if authorizationURL.Query().Get("client_id") != "dynamic-client-id" {
				t.Fatal("sign-in retained the old OAuth client")
			}
			var callback *httptest.ResponseRecorder
			if test.retry {
				done := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					done <- respond(start.AuthURL, "approved-code")
				}()
				select {
				case <-saving:
				case <-ctx.Done():
					t.Fatal("replacement callback did not reach connection save")
				}
				_, err := startConnect()
				close(saveAllowed)
				if err == nil || !strings.Contains(err.Error(), "already in progress") {
					t.Fatalf("retry accepted after consent: %v", err)
				}
				callback = <-done
			} else {
				callback = respond(start.AuthURL, "approved-code")
			}
			if unrelatedRequests.Load() != before {
				t.Fatal("sign-in reconciled an unrelated server")
			}
			grant, ok, loadErr := store.LoadToken(ctx, targetServer.TokenID())
			if loadErr != nil || !ok || grant.ClientID != "dynamic-client-id" || grant.RefreshToken != "refresh-token-456" {
				t.Fatalf("reconnect did not persist new grant/client: %+v, %v", grant, loadErr)
			}
			if test.unavailable {
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
			if existing != nil {
				result, err := existing.CallTool(ctx, &mcpsdk.CallToolParams{Name: "echo", Arguments: map[string]any{"value": "after sign-in"}})
				if err != nil || result.IsError || lastAuthorization.Load() != "Bearer access-token-123" {
					t.Fatalf("warmed client retained credentials after Connections sign-in: %+v, %v", result, err)
				}
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
