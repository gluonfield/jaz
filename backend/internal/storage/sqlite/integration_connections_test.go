package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestIntegrationConnectionRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connection := integrations.Connection{
		ID:          "gmail:default",
		Provider:    "gmail",
		AccountID:   "augustinas@example.com",
		AccountName: "Augustinas",
		Alias:       "personal",
		Scopes:      []string{"https://www.googleapis.com/auth/gmail.modify"},
	}
	if err := store.SaveConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.LoadConnection(context.Background(), connection.ID)
	if err != nil || !ok {
		t.Fatalf("loaded ok=%v err=%v", ok, err)
	}
	if loaded.ID != connection.ID ||
		loaded.Provider != connection.Provider ||
		loaded.AccountID != connection.AccountID ||
		loaded.AccountName != connection.AccountName ||
		loaded.Alias != connection.Alias ||
		len(loaded.Scopes) != 1 ||
		loaded.Scopes[0] != connection.Scopes[0] {
		t.Fatalf("loaded = %#v", loaded)
	}

	connections, err := store.ListConnections(context.Background(), "gmail")
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ID != connection.ID {
		t.Fatalf("connections = %#v", connections)
	}
}

func TestSaveOAuthConnectionPersistsTokenAndConnection(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connection := integrations.Connection{
		ID:        "gmail:default",
		Provider:  "gmail",
		AccountID: "augustinas@example.com",
		Alias:     "default",
		Scopes:    []string{"scope"},
	}
	token := integrationoauth.Token{AccessToken: "access", RefreshToken: "refresh", Scopes: []string{"scope"}}
	if err := store.SaveOAuthConnection(context.Background(), token, connection); err != nil {
		t.Fatal(err)
	}
	loadedToken, ok, err := store.LoadToken(context.Background(), connection.ID)
	if err != nil || !ok {
		t.Fatalf("token ok=%v err=%v", ok, err)
	}
	if loadedToken.AccessToken != token.AccessToken || loadedToken.RefreshToken != token.RefreshToken {
		t.Fatalf("token = %#v", loadedToken)
	}
	loadedConnection, ok, err := store.LoadConnection(context.Background(), connection.ID)
	if err != nil || !ok {
		t.Fatalf("connection ok=%v err=%v", ok, err)
	}
	if loadedConnection.AccountID != connection.AccountID || loadedConnection.Provider != connection.Provider {
		t.Fatalf("connection = %#v", loadedConnection)
	}
}

func TestDeleteConnectionDeletesTokenAndConnection(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connection := integrations.Connection{
		ID:        "gmail:personal",
		Provider:  "gmail",
		AccountID: "augustinas@example.com",
		Alias:     "personal",
		Scopes:    []string{"scope"},
	}
	token := integrationoauth.Token{AccessToken: "access", RefreshToken: "refresh", Scopes: []string{"scope"}}
	if err := store.SaveOAuthConnection(context.Background(), token, connection); err != nil {
		t.Fatal(err)
	}
	cursor := integrations.Cursor{Kind: "gmail.sync", Value: json.RawMessage(`{"history_id":"h1"}`)}
	if err := store.SaveIntegrationCursor(context.Background(), connection.ID, cursor); err != nil {
		t.Fatal(err)
	}
	ok, err := store.DeleteConnection(context.Background(), connection.ID)
	if err != nil || !ok {
		t.Fatalf("delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.LoadConnection(context.Background(), connection.ID); err != nil || ok {
		t.Fatalf("connection after delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.LoadToken(context.Background(), connection.ID); err != nil || ok {
		t.Fatalf("token after delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.LoadIntegrationCursor(context.Background(), connection.ID, cursor.Kind); err != nil || ok {
		t.Fatalf("cursor after delete ok=%v err=%v", ok, err)
	}
	ok, err = store.DeleteConnection(context.Background(), connection.ID)
	if err != nil || ok {
		t.Fatalf("second delete ok=%v err=%v", ok, err)
	}
}

func TestIntegrationCursorRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connection := integrations.Connection{
		ID:        "gmail:personal",
		Provider:  "gmail",
		AccountID: "augustinas@example.com",
		Alias:     "personal",
		Scopes:    []string{"scope"},
	}
	if err := store.SaveConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	cursor := integrations.Cursor{Kind: "gmail.sync", Value: json.RawMessage(`{"backfill_page_token":"next"}`)}
	if err := store.SaveIntegrationCursor(context.Background(), connection.ID, cursor); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.LoadIntegrationCursor(context.Background(), connection.ID, cursor.Kind)
	if err != nil || !ok {
		t.Fatalf("cursor ok=%v err=%v", ok, err)
	}
	if loaded.Kind != cursor.Kind || string(loaded.Value) != string(cursor.Value) {
		t.Fatalf("cursor = %#v", loaded)
	}
	loadedConnection, ok, err := store.LoadConnection(context.Background(), connection.ID)
	if err != nil || !ok {
		t.Fatalf("connection ok=%v err=%v", ok, err)
	}
	if loadedConnection.LastSyncedAt == nil || loadedConnection.LastSyncedAt.IsZero() {
		t.Fatalf("last synced at = %#v", loadedConnection.LastSyncedAt)
	}
	connections, err := store.ListConnections(context.Background(), connection.Provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].LastSyncedAt == nil || connections[0].LastSyncedAt.IsZero() {
		t.Fatalf("connections = %#v", connections)
	}
}

func TestDeleteConnectionKeepsTokenWithoutConnection(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connectionID := "gmail:token-only"
	token := integrationoauth.Token{AccessToken: "access", RefreshToken: "refresh"}
	if err := store.SaveToken(context.Background(), connectionID, token); err != nil {
		t.Fatal(err)
	}
	ok, err := store.DeleteConnection(context.Background(), connectionID)
	if err != nil || ok {
		t.Fatalf("delete ok=%v err=%v", ok, err)
	}
	loaded, ok, err := store.LoadToken(context.Background(), connectionID)
	if err != nil || !ok {
		t.Fatalf("token after missing connection delete ok=%v err=%v", ok, err)
	}
	if loaded.AccessToken != token.AccessToken || loaded.RefreshToken != token.RefreshToken {
		t.Fatalf("token = %#v", loaded)
	}
}
