package connections

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	core "github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/deployink"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestConnectStartDoesNotLeakMCPConnectionInternals(t *testing.T) {
	refresher := newTestMCPRefresher()
	store := &connectionStore{}
	authorizer := &mcpConnectionAuthorizer{authURL: "https://deployink.com/oauth"}
	service := core.NewConnectService(core.NewCatalog(nil), nil, nil, nil, core.NewMCPConnectionConnector(store, authorizer))
	handler := NewConnectHandler(service, nil, nil, refresher, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/connections/plugins/deployink/connect", nil)
	req.SetPathValue("id", deployink.ProviderID)
	res := httptest.NewRecorder()

	handler.Start(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "MCPServersChanged") {
		t.Fatalf("internal change flag leaked in response: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "mcp") {
		t.Fatalf("MCP implementation leaked in response: %s", res.Body.String())
	}
	refresher.notCalled(t)
}

func TestPluginDisconnectRefreshesMCPWhenConnectionBackedMCPChanges(t *testing.T) {
	refresher := newTestMCPRefresher()
	store := connectionStore{connection: integrations.Connection{
		ID:          "deployink:mcp-deployink-com",
		Provider:    deployink.ProviderID,
		AccountID:   deployink.RemoteMCPURL,
		AccountName: deployink.ProviderName,
	}}
	service := core.NewService(core.NewCatalog(nil), &store, nil)
	handler := NewPluginHandler(service, refresher, "")
	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/accounts/deployink:mcp-deployink-com", nil)
	req.SetPathValue("id", "deployink:mcp-deployink-com")
	res := httptest.NewRecorder()

	handler.Disconnect(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	refresher.wait(t)
}

func TestZoomPublicSignInUsesSameAvailabilityAsCatalogue(t *testing.T) {
	catalog := core.NewCatalog(map[string]mcpconfig.OAuthConfig{"zoom": {PublicClientID: "registered-public"}})
	store := &connectionStore{connection: integrations.Connection{ID: "zoom:account", Provider: "zoom"}}
	service := core.NewService(catalog, store, nil)
	for _, test := range []struct {
		name       string
		peer       string
		forwarded  string
		publicURL  string
		canConnect bool
	}{
		{name: "direct local", peer: "127.0.0.1:1234", canConnect: true},
		{name: "remote", peer: "192.0.2.1:1234"},
		{name: "forwarded", peer: "127.0.0.1:1234", forwarded: "client.example"},
		{name: "public backend", peer: "127.0.0.1:1234", publicURL: "https://jaz.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &mcpConnectionAuthorizer{authURL: "https://zoom.us/oauth/authorize"}
			connectService := core.NewConnectService(catalog, nil, nil, nil, core.NewMCPConnectionConnector(store, authorizer))
			plugins := NewPluginHandler(service, nil, test.publicURL)
			connect := NewConnectHandler(connectService, nil, nil, nil, test.publicURL)
			request := func(method, path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, "http://127.0.0.1:5299/v1/connections/plugins"+path, nil)
				req.RemoteAddr = test.peer
				req.Header.Set("X-Forwarded-Host", test.forwarded)
				req.SetPathValue("id", "zoom")
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}
			var listing struct {
				Plugins []integrations.Plugin `json:"plugins"`
			}
			listed := request(http.MethodGet, "", plugins.List)
			if err := json.Unmarshal(listed.Body.Bytes(), &listing); err != nil || listed.Code != http.StatusOK {
				t.Fatalf("catalogue response = %d, %v", listed.Code, err)
			}
			var detail integrations.Plugin
			details := request(http.MethodGet, "/zoom", plugins.Get)
			if err := json.Unmarshal(details.Body.Bytes(), &detail); err != nil || details.Code != http.StatusOK || detail.Connection == nil || detail.Connection.Status != integrations.PluginConnectionStatusConnected {
				t.Fatalf("Zoom detail = %d, %v", details.Code, err)
			}
			zoom := slices.IndexFunc(listing.Plugins, func(plugin integrations.Plugin) bool { return plugin.ID == "zoom" })
			if zoom < 0 || listing.Plugins[zoom].Implementation != detail.Implementation {
				t.Fatal("Zoom list and detail availability differ")
			}
			start := request(http.MethodPost, "/zoom/connect", connect.Start)
			if detail.Available() != test.canConnect || (start.Code == http.StatusOK) != test.canConnect || (authorizer.onAuthorized != nil) != test.canConnect {
				t.Fatalf("Zoom availability = %v, start status = %d", detail.Implementation, start.Code)
			}
			if !test.canConnect && (start.Code != http.StatusBadRequest || detail.Implementation.Reason == "" || !strings.Contains(start.Body.String(), detail.Implementation.Reason)) {
				t.Fatalf("Zoom start did not return its unavailable reason: %d %s", start.Code, start.Body.String())
			}
		})
	}
}

type testMCPRefresher struct {
	called chan struct{}
}

func newTestMCPRefresher() *testMCPRefresher {
	return &testMCPRefresher{called: make(chan struct{}, 1)}
}

func (r *testMCPRefresher) Refresh(context.Context) {
	select {
	case r.called <- struct{}{}:
	default:
	}
}

func (r *testMCPRefresher) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.called:
	case <-time.After(time.Second):
		t.Fatal("expected MCP refresh")
	}
}

func (r *testMCPRefresher) notCalled(t *testing.T) {
	t.Helper()
	select {
	case <-r.called:
		t.Fatal("unexpected MCP refresh")
	case <-time.After(50 * time.Millisecond):
	}
}

type connectionStore struct {
	connection  integrations.Connection
	connections []integrations.Connection
}

func (s *connectionStore) SaveConnection(_ context.Context, connection integrations.Connection) error {
	s.connections = append(s.connections, connection)
	return nil
}

func (s *connectionStore) LoadConnection(_ context.Context, id string) (integrations.Connection, bool, error) {
	if s.connection.ID == id {
		return s.connection, true, nil
	}
	return integrations.Connection{}, false, nil
}

func (s *connectionStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	if s.connection.Provider == provider {
		return []integrations.Connection{s.connection}, nil
	}
	return nil, nil
}

func (s *connectionStore) DeleteConnection(_ context.Context, id string) (bool, error) {
	if s.connection.ID != id {
		return false, nil
	}
	s.connection = integrations.Connection{}
	return true, nil
}

type mcpConnectionAuthorizer struct {
	authURL      string
	onAuthorized func(context.Context) error
}

func (a *mcpConnectionAuthorizer) AuthorizeWithHook(_ context.Context, _ mcpconfig.Server, _ mcpconfig.AuthorizeOptions, onAuthorized func(context.Context) error) mcpconfig.ServerStatus {
	a.onAuthorized = onAuthorized
	return mcpconfig.ServerStatus{Status: "needs_auth", AuthURL: a.authURL}
}
