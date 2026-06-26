package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestConnectServiceReportsMissingQRProviderForSessionPlugin(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService())

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
	service := NewConnectService(catalog, nil, qr)

	_, err := service.Start(context.Background(), "matrix", "")
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectServiceStartsChatQRProviders(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService(
		fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)},
		fakeQRProvider{provider: whatsapp.ProviderID, expires: time.Now().Add(time.Minute)},
	))

	for _, provider := range []string{telegram.ProviderID, whatsapp.ProviderID} {
		start, err := service.Start(context.Background(), provider, "")
		if err != nil {
			t.Fatalf("%s start err = %v", provider, err)
		}
		if start.Type != "qr" || start.QR == nil || start.QR.Provider != provider || start.QR.Code == "" {
			t.Fatalf("%s start = %#v", provider, start)
		}
	}
}

func TestConnectServiceRejectsUnknownProvider(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService())
	if _, err := service.Start(context.Background(), "missing", ""); err == nil {
		t.Fatal("expected error")
	}
}
