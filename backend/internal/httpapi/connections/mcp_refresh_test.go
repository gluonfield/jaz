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

func TestConnectStartRefreshesMCPWhenServerChanges(t *testing.T) {
	refresher := newTestMCPRefresher()
	store := &remoteMCPStore{}
	service := core.NewConnectService(core.NewCatalog(), nil, nil, core.NewRemoteMCPConnector(store))
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
	refresher.wait(t)
}

func TestPluginDisconnectRefreshesMCPWhenServerChanges(t *testing.T) {
	refresher := newTestMCPRefresher()
	store := &remoteMCPStore{servers: []mcpconfig.Server{{
		ID:        "mcp_deployink",
		Name:      deployink.ProviderName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       deployink.RemoteMCPURL,
		Enabled:   true,
	}}}
	service := core.NewService(core.NewCatalog(), connectionStore{}, core.NewRemoteMCPConnector(store))
	handler := NewPluginHandler(service, refresher)
	req := httptest.NewRequest(http.MethodDelete, "/v1/connections/accounts/mcp_deployink", nil)
	req.SetPathValue("id", "mcp_deployink")
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

type connectionStore struct{}

func (connectionStore) LoadConnection(context.Context, string) (integrations.Connection, bool, error) {
	return integrations.Connection{}, false, nil
}

func (connectionStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return nil, nil
}

func (connectionStore) DeleteConnection(context.Context, string) (bool, error) {
	return false, nil
}

type remoteMCPStore struct {
	servers []mcpconfig.Server
	nextID  int
}

func (s *remoteMCPStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func (s *remoteMCPStore) CreateMCPServer(input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	s.nextID++
	server := mcpconfig.Server{
		ID:        "mcp_test",
		Name:      input.Name,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       input.URL,
		Enabled:   input.Enabled,
	}
	s.servers = append(s.servers, server)
	return server, nil
}

func (s *remoteMCPStore) UpdateMCPServer(id string, input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	for i := range s.servers {
		if s.servers[i].ID != id {
			continue
		}
		s.servers[i].Name = input.Name
		s.servers[i].URL = input.URL
		s.servers[i].Enabled = input.Enabled
		return s.servers[i], nil
	}
	return s.CreateMCPServer(input)
}

func (s *remoteMCPStore) DeleteMCPServer(id string) error {
	for i := range s.servers {
		if s.servers[i].ID == id {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			return nil
		}
	}
	return nil
}
