package sqlite

import (
	"context"
	"testing"
	"time"

	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestIntegrationOAuthTokenRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	want := integrationoauth.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC),
		ClientID:     "client",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"read"},
	}
	if err := store.SaveToken(context.Background(), "gmail:primary", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadToken(context.Background(), "gmail:primary")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("token not found")
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || got.ClientID != want.ClientID || got.TokenURL != want.TokenURL || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("token = %#v, want %#v", got, want)
	}
}
