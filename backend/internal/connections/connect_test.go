package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestConnectServiceReportsMissingQRProviderForSessionPlugin(t *testing.T) {
	service := NewConnectService(NewCatalog(nil), nil, NewQRService(), nil)

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
	service := NewConnectService(catalog, nil, qr, nil)

	_, err := service.Start(context.Background(), "matrix", StartOptions{})
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectServiceStartsChatQRProviders(t *testing.T) {
	service := NewConnectService(NewCatalog(nil), nil, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	), nil)

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
	service := NewConnectService(NewCatalog(nil), nil, NewQRService(), nil)
	if _, err := service.Start(context.Background(), "missing", StartOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestConnectServiceStartsGitPluginMCPAuthorization(t *testing.T) {
	store := &mcpConnectionStore{}
	authorizer := &fakeMCPConnectionAuthorizer{authURL: "https://zoom.us/oauth/authorize"}
	client := mcpconfig.OAuthConfig{ClientID: "registered-client", ClientSecretEnvVar: "ZOOM_TEST_SECRET", UseAppScopes: true}
	t.Setenv(client.ClientSecretEnvVar, "registered-secret")
	catalog := NewCatalog(map[string]mcpconfig.OAuthConfig{"zoom": client})
	service := NewConnectService(catalog, nil, nil, NewMCPConnectionConnector(store, authorizer))

	result, err := service.StartMCP(context.Background(), "zoom", "plugin:zoom:bWFpbg", "Zoom", "https://mcp.zoom.us/mcp/zoom/streamable", mcpconfig.AuthorizeOptions{
		RedirectURL: "https://jaz.example.com/v1/mcp/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Start.Type != "oauth" || result.Start.AuthURL != authorizer.authURL || result.MCPServersChanged {
		t.Fatalf("result = %#v", result)
	}
	if authorizer.server.ID != "plugin:zoom:bWFpbg" || authorizer.server.OAuth != client || authorizer.server.URL != "https://mcp.zoom.us/mcp/zoom/streamable" {
		t.Fatalf("server = %#v", authorizer.server)
	}
	if err := authorizer.onAuthorized(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.connections) != 1 || store.connections[0].Provider != "zoom" || store.connections[0].AccountName != "Zoom" {
		t.Fatalf("connections = %#v", store.connections)
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
