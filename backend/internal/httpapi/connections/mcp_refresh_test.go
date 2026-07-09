package connections

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	service := core.NewConnectService(core.NewCatalog(), nil, nil, nil, core.NewMCPConnectionConnector(store, authorizer))
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
	service := core.NewService(core.NewCatalog(), &store, nil)
	handler := NewPluginHandler(service, refresher)
	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/accounts/deployink:mcp-deployink-com", nil)
	req.SetPathValue("id", "deployink:mcp-deployink-com")
	res := httptest.NewRecorder()

	handler.Disconnect(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	refresher.wait(t)
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
