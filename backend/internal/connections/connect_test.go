package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connectors/deployink"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestConnectServiceReportsMissingQRProviderForSessionPlugin(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(), nil, nil)

	_, err := service.Start(context.Background(), whatsapp.ProviderID, StartOptions{})
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectServiceDelegatesSessionAuthToQRService(t *testing.T) {
	qr := NewQRService()
	catalog := &Catalog{plugins: []integrations.Plugin{{
		ID: "matrix",
		Provider: integrations.Provider{
			ID:   "matrix",
			Name: "Matrix",
		},
		Auth: []integrations.AuthOption{{Kind: integrations.AuthKindSession}},
		Implementation: integrations.Implementation{
			Status: "available",
		},
	}}}
	service := NewConnectService(catalog, nil, qr, nil, nil)

	_, err := service.Start(context.Background(), "matrix", StartOptions{})
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectServiceStartsChatQRProviders(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	), nil, nil)

	for _, provider := range []string{telegram.ProviderID, whatsapp.ProviderID} {
		result, err := service.Start(context.Background(), provider, StartOptions{})
		if err != nil {
			t.Fatalf("%s start err = %v", provider, err)
		}
		start := result.Start
		if start.Type != "qr" || start.QR == nil || start.QR.Provider != provider || start.QR.Code == "" {
			t.Fatalf("%s start = %#v", provider, start)
		}
	}
}

func TestConnectServiceRejectsUnknownProvider(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(), nil, nil)
	if _, err := service.Start(context.Background(), "missing", StartOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestConnectServiceAddsRemoteMCPServer(t *testing.T) {
	store := &remoteMCPStore{}
	catalog := &Catalog{plugins: []integrations.Plugin{remoteMCPPlugin()}}
	service := NewConnectService(catalog, nil, nil, NewRemoteMCPConnector(store), nil)

	result, err := service.Start(context.Background(), "docs", StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	start := result.Start
	if start.Type != "mcp" || start.MCP == nil || start.MCP.URL != "https://mcp.example.com" {
		t.Fatalf("start = %#v", start)
	}
	if !result.MCPServersChanged {
		t.Fatalf("result = %#v, want MCPServersChanged", result)
	}
	if len(store.servers) != 1 {
		t.Fatalf("servers = %#v", store.servers)
	}
}

func TestConnectServiceStartsDeployinkAsConnectionBackedMCP(t *testing.T) {
	store := &mcpConnectionStore{}
	authorizer := &fakeMCPConnectionAuthorizer{authURL: "https://deployink.com/oauth"}
	service := NewConnectService(NewCatalog(), nil, nil, nil, NewMCPConnectionConnector(store, authorizer))

	result, err := service.Start(context.Background(), deployink.ProviderID, StartOptions{
		MCPRedirectURL: "https://jaz.example.com/v1/mcp/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Start.Type != "oauth" || result.Start.AuthURL != authorizer.authURL || result.MCPServersChanged {
		t.Fatalf("result = %#v", result)
	}
	if len(store.connections) != 0 {
		t.Fatalf("connections before callback = %#v", store.connections)
	}
	if authorizer.onAuthorized == nil {
		t.Fatal("missing authorization callback")
	}
	if err := authorizer.onAuthorized(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.connections) != 1 {
		t.Fatalf("connections after callback = %#v", store.connections)
	}
	connection := store.connections[0]
	if connection.Provider != deployink.ProviderID || connection.AccountName != deployink.ProviderName {
		t.Fatalf("connection = %#v", connection)
	}
	if authorizer.server.ID != connection.ID || authorizer.server.URL != deployink.RemoteMCPURL {
		t.Fatalf("server = %#v connection = %#v", authorizer.server, connection)
	}
	if authorizer.options.RedirectURL != "https://jaz.example.com/v1/mcp/oauth/callback" || !authorizer.options.ReturnAuthURL {
		t.Fatalf("authorize options = %#v", authorizer.options)
	}
}

func remoteMCPPlugin() integrations.Plugin {
	return integrations.Plugin{
		ID:   "docs",
		Name: "Docs",
		Provider: integrations.Provider{
			ID:   "docs",
			Name: "Docs",
		},
		Auth: []integrations.AuthOption{{Kind: integrations.AuthKindRemoteMCP}},
		RemoteMCP: &integrations.RemoteMCP{
			URL: "https://mcp.example.com",
		},
		Implementation: integrations.Implementation{
			Status: "available",
		},
	}
}

type mcpConnectionStore struct {
	connections []integrations.Connection
}

func (s *mcpConnectionStore) SaveConnection(_ context.Context, connection integrations.Connection) error {
	s.connections = append(s.connections, connection)
	return nil
}

type fakeMCPConnectionAuthorizer struct {
	authURL      string
	server       mcpconfig.Server
	options      mcpconfig.AuthorizeOptions
	onAuthorized func(context.Context) error
}

func (a *fakeMCPConnectionAuthorizer) AuthorizeWithHook(_ context.Context, server mcpconfig.Server, options mcpconfig.AuthorizeOptions, onAuthorized func(context.Context) error) mcpconfig.ServerStatus {
	a.server = server
	a.options = options
	a.onAuthorized = onAuthorized
	return mcpconfig.ServerStatus{Status: "needs_auth", AuthURL: a.authURL}
}
