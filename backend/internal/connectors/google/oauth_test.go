package google

import "testing"

func TestOAuthClientConfigCredentials(t *testing.T) {
	defaults, err := (OAuthClientConfig{}).Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if defaults != DefaultOAuthClientCredentials() {
		t.Fatalf("defaults = %#v", defaults)
	}

	custom, err := (OAuthClientConfig{
		ClientID:     " custom-client ",
		ClientSecret: " custom-secret ",
	}).Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if custom.ClientID != "custom-client" || custom.ClientSecret != "custom-secret" {
		t.Fatalf("custom = %#v", custom)
	}

	if _, err := (OAuthClientConfig{ClientID: "custom-client"}).Credentials(); err == nil {
		t.Fatal("expected partial override error")
	}
}
