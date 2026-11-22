package connections_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

func TestDisconnectCannotBeUndoneByInflightRefresh(t *testing.T) {
	for _, managedMCP := range []bool{false, true} {
		t.Run(map[bool]string{false: "native-oauth", true: "managed-mcp"}[managedMCP], func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"stale","token_type":"Bearer","expires_in":3600}`))
			}))
			defer upstream.Close()
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			account := integrations.Connection{ID: "fixture:account", Provider: "fixture", AccountID: "account"}
			if err := store.SaveConnection(t.Context(), account); err != nil {
				t.Fatal(err)
			}
			tokenID := account.ID
			if managedMCP {
				tokenID = mcpconfig.OAuthConnectionID(account.ID)
			}
			if err := store.SaveToken(t.Context(), tokenID, integrationoauth.Token{AccessToken: "expired", RefreshToken: "old", TokenURL: upstream.URL, AuthStyle: int(oauth2.AuthStyleInParams), Expiry: time.Now().Add(-time.Hour)}); err != nil {
				t.Fatal(err)
			}
			source, err := (integrationoauth.Refresher{Store: store}).TokenSource(t.Context(), tokenID)
			if err != nil {
				t.Fatal(err)
			}
			refreshed := make(chan error, 1)
			go func() {
				_, err := source.Token()
				refreshed <- err
			}()
			<-started
			disconnected := make(chan error, 1)
			go func() {
				_, err := connections.NewService(connections.NewCatalog(nil), store).DisconnectAccount(t.Context(), account.ID)
				disconnected <- err
			}()
			early := false
			select {
			case err := <-disconnected:
				early = true
				t.Errorf("grant mutation completed before in-flight refresh: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(release)
			if err := <-refreshed; err != nil {
				t.Fatal(err)
			}
			if !early {
				if err := <-disconnected; err != nil {
					t.Fatal(err)
				}
			}
			if _, ok, err := store.LoadToken(t.Context(), tokenID); err != nil || ok {
				t.Fatalf("refresh resurrected deleted grant: exists=%v, %v", ok, err)
			}
			if _, err := source.Token(); !errors.Is(err, integrationoauth.ErrTokenNotFound) {
				t.Fatalf("deleted grant remains usable: %v", err)
			}
		})
	}
}
