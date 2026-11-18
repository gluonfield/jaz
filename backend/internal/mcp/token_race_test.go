package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
	"golang.org/x/oauth2"
)

func TestInteractiveGrantReplacementWinsOverInflightRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"stale-refresh-result","refresh_token":"stale-rotation","token_type":"Bearer","expires_in":3600}`))
	}))
	defer upstream.Close()
	server := mcpconfig.Server{ID: "managed-account"}
	store := newMemTokenStore()
	old := integrationoauth.Token{AccessToken: "expired", RefreshToken: "old-refresh", ClientID: "old-client", TokenURL: upstream.URL, AuthStyle: int(oauth2.AuthStyleInParams), Expiry: time.Now().Add(-time.Hour)}
	if err := store.SaveToken(t.Context(), server.TokenID(), old); err != nil {
		t.Fatal(err)
	}
	source, err := (integrationoauth.Refresher{Store: store}).TokenSource(t.Context(), server.TokenID())
	if err != nil {
		t.Fatal(err)
	}
	refreshed := make(chan error, 1)
	go func() {
		_, err := source.Token()
		refreshed <- err
	}()
	<-started
	newGrant := integrationoauth.Token{AccessToken: "approved", RefreshToken: "approved-refresh", ClientID: "approved-client"}
	saved := make(chan error, 1)
	go func() {
		saved <- newOAuthHandler(server, store, http.DefaultClient, nil).saveToken(t.Context(), newGrant)
	}()
	early := false
	select {
	case err := <-saved:
		early = true
		t.Errorf("grant mutation completed before in-flight refresh: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if !early {
		if err := <-saved; err != nil {
			t.Fatal(err)
		}
	}
	stored, ok, err := store.LoadToken(t.Context(), server.TokenID())
	if err != nil || !ok || stored.AccessToken != newGrant.AccessToken || stored.ClientID != newGrant.ClientID || stored.RefreshToken != newGrant.RefreshToken {
		t.Fatalf("old refresh overwrote approval: %+v, %v", stored, err)
	}
	current, err := source.Token()
	if err != nil || current.AccessToken != newGrant.AccessToken {
		t.Fatalf("old source retained stale grant: %+v, %v", current, err)
	}
}
