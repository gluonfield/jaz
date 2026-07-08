package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connectors/deployink"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestConnectServiceReportsMissingQRProviderForSessionPlugin(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(), nil)

	_, err := service.Start(context.Background(), whatsapp.ProviderID, "")
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

	_, err := service.Start(context.Background(), "matrix", "")
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectServiceStartsChatQRProviders(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	), nil)

	for _, provider := range []string{telegram.ProviderID, whatsapp.ProviderID} {
		result, err := service.Start(context.Background(), provider, "")
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
	service := NewConnectService(NewCatalog(), nil, NewQRService(), nil)
	if _, err := service.Start(context.Background(), "missing", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestConnectServiceAddsRemoteMCPServer(t *testing.T) {
	store := &remoteMCPStore{}
	service := NewConnectService(NewCatalog(), nil, nil, NewRemoteMCPConnector(store))

	result, err := service.Start(context.Background(), deployink.ProviderID, "")
	if err != nil {
		t.Fatal(err)
	}
	start := result.Start
	if start.Type != "mcp" || start.MCP == nil || start.MCP.URL != deployink.RemoteMCPURL {
		t.Fatalf("start = %#v", start)
	}
	if !result.MCPServersChanged {
		t.Fatalf("result = %#v, want MCPServersChanged", result)
	}
	if len(store.servers) != 1 {
		t.Fatalf("servers = %#v", store.servers)
	}
}
