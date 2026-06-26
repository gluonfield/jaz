package gmail

import "testing"

func TestOAuthClientConfigCredentials(t *testing.T) {
	defaults, err := (OAuthClientConfig{}).Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.ClientID != OAuthClientID || defaults.ClientSecret != OAuthClientSecret {
		t.Fatalf("defaults = %#v", defaults)
	}

	custom, err := (OAuthClientConfig{
		ClientID:     " custom-client.apps.googleusercontent.com ",
		ClientSecret: " custom-secret ",
	}).Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if custom.ClientID != "custom-client.apps.googleusercontent.com" || custom.ClientSecret != "custom-secret" {
		t.Fatalf("custom = %#v", custom)
	}

	if _, err := (OAuthClientConfig{ClientID: "custom-client.apps.googleusercontent.com"}).Credentials(); err == nil {
		t.Fatal("expected partial override error")
	}
}
