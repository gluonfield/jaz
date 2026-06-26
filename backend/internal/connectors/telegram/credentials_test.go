package telegram

import "testing"

func TestCredentialsUsesBundledClient(t *testing.T) {
	defaults, ok, err := ParseCredentials(" 123 ", " bundled-hash ")
	if err != nil || !ok || defaults.APIID != 123 || defaults.APIHash != "bundled-hash" {
		t.Fatalf("defaults ok=%v credentials=%#v err=%v", ok, defaults, err)
	}
}

func TestCredentialsUsesEnvironmentFallback(t *testing.T) {
	withBundledCredentials(t, "", "")
	t.Setenv(EnvAppID, "456")
	t.Setenv(EnvAppHash, "env-hash")

	credentials, ok, err := Credentials()
	if err != nil || !ok || credentials.APIID != 456 || credentials.APIHash != "env-hash" {
		t.Fatalf("credentials ok=%v value=%#v err=%v", ok, credentials, err)
	}
}

func TestCredentialsBundledClientOverridesEnvironment(t *testing.T) {
	withBundledCredentials(t, "789", "bundled-hash")
	t.Setenv(EnvAppID, "456")
	t.Setenv(EnvAppHash, "env-hash")

	credentials, ok, err := Credentials()
	if err != nil || !ok || credentials.APIID != 789 || credentials.APIHash != "bundled-hash" {
		t.Fatalf("credentials ok=%v value=%#v err=%v", ok, credentials, err)
	}
}

func TestCredentialsWithoutBundledClient(t *testing.T) {
	_, ok, err := ParseCredentials("", "")
	if err != nil || ok {
		t.Fatalf("empty config ok=%v err=%v", ok, err)
	}
}

func TestCredentialsRejectsInvalidEnvironmentFallback(t *testing.T) {
	withBundledCredentials(t, "", "")
	t.Setenv(EnvAppID, "456")

	_, ok, err := Credentials()
	if err == nil || ok {
		t.Fatalf("credentials ok=%v err=%v", ok, err)
	}
}

func withBundledCredentials(t *testing.T, id, hash string) {
	t.Helper()
	originalID, originalHash := bundledClientID, bundledClientHash
	bundledClientID, bundledClientHash = id, hash
	t.Cleanup(func() {
		bundledClientID, bundledClientHash = originalID, originalHash
	})
}

func TestCredentialsRejectsInvalidBundledClient(t *testing.T) {
	for _, tc := range []struct {
		id   string
		hash string
	}{
		{id: "123"},
		{hash: "hash"},
		{id: "abc", hash: "hash"},
		{id: "0", hash: "hash"},
	} {
		if _, ok, err := ParseCredentials(tc.id, tc.hash); err == nil || ok {
			t.Fatalf("ParseCredentials(%q, %q) ok=%v err=%v", tc.id, tc.hash, ok, err)
		}
	}
}
