package gmail

import (
	"strings"
	"testing"
)

func TestOAuthClientConfigCredentials(t *testing.T) {
	if _, err := (OAuthClientConfig{}).Credentials(); err == nil {
		t.Fatal("expected missing credentials error")
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

func TestConnectionID(t *testing.T) {
	first, err := ConnectionID(" Augustinas.Example@gmail.com ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ConnectionID("augustinas-example@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("connection ids should include a hash suffix: %q", first)
	}
	if !strings.HasPrefix(first, "gmail:augustinas-example-gmail-com-") {
		t.Fatalf("connection id = %q", first)
	}
	if _, err := ConnectionID(" "); err == nil {
		t.Fatal("expected empty account error")
	}
}
