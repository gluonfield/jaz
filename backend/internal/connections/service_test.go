package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestServiceReportsGmailConnectionState(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusNotConnected || len(plugin.Connection.Accounts) != 0 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
}

func TestServiceReturnsSavedGmailAccounts(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{
		connections: []integrations.Connection{{
			ID:          gmailconnector.OAuthConnectionID,
			Provider:    gmailconnector.ProviderID,
			AccountID:   "augustinas@example.com",
			AccountName: "Augustinas",
			Alias:       "personal",
			Scopes:      []string{gmailconnector.ScopeModify},
		}},
	}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), gmailconnector.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Connection == nil || plugin.Connection.Status != integrations.PluginConnectionStatusConnected || len(plugin.Connection.Accounts) != 1 {
		t.Fatalf("connection = %#v", plugin.Connection)
	}
	if plugin.Connection.Accounts[0].AccountID != "augustinas@example.com" || plugin.Connection.Accounts[0].Alias != "personal" {
		t.Fatalf("account = %#v", plugin.Connection.Accounts[0])
	}
}

func TestServiceReportsMissingChatSessionAdapter(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService())
	plugin, ok, err := service.Plugin(context.Background(), whatsapp.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Implementation.Status != "adapter_required" {
		t.Fatalf("implementation = %#v", plugin.Implementation)
	}
}

func TestServiceMarksSessionPluginAvailableWhenQRProviderExists(t *testing.T) {
	service := NewService(NewCatalog(), &serviceStore{}, NewQRService(fakeQRProvider{provider: telegram.ProviderID, expires: time.Now().Add(time.Minute)}))
	plugin, ok, err := service.Plugin(context.Background(), telegram.ProviderID)
	if err != nil || !ok {
		t.Fatalf("plugin ok=%v err=%v", ok, err)
	}
	if plugin.Implementation.Status != "available" {
		t.Fatalf("implementation = %#v", plugin.Implementation)
	}
}

func TestServiceDisconnectAccount(t *testing.T) {
	store := serviceStore{
		connections: []integrations.Connection{{
			ID:       "gmail:personal",
			Provider: gmailconnector.ProviderID,
			Alias:    "personal",
		}},
	}
	service := NewService(NewCatalog(), &store, NewQRService())
	if err := service.DisconnectAccount(context.Background(), " gmail:personal "); err != nil {
		t.Fatal(err)
	}
	if len(store.connections) != 0 {
		t.Fatalf("connections = %#v", store.connections)
	}
	if err := service.DisconnectAccount(context.Background(), "gmail:missing"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

type serviceStore struct {
	connections []integrations.Connection
}

func (s serviceStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	var out []integrations.Connection
	for _, connection := range s.connections {
		if connection.Provider == provider {
			out = append(out, connection)
		}
	}
	return out, nil
}

func (s *serviceStore) DeleteConnection(_ context.Context, id string) (bool, error) {
	for i, connection := range s.connections {
		if connection.ID == id {
			s.connections = append(s.connections[:i], s.connections[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
