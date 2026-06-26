package connections

import (
	"context"
	"errors"
	"testing"

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

func TestConnectServiceRejectsUnknownProvider(t *testing.T) {
	service := NewConnectService(NewCatalog(), nil, NewQRService())
	if _, err := service.Start(context.Background(), "missing", ""); err == nil {
		t.Fatal("expected error")
	}
}
