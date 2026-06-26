package telegram

import "testing"

func TestCredentialsUsesBundledClient(t *testing.T) {
	defaults, ok, err := credentialsFrom(" 123 ", " bundled-hash ")
	if err != nil || !ok || defaults.APIID != 123 || defaults.APIHash != "bundled-hash" {
		t.Fatalf("defaults ok=%v credentials=%#v err=%v", ok, defaults, err)
	}
}

func TestCredentialsWithoutBundledClient(t *testing.T) {
	_, ok, err := credentialsFrom("", "")
	if err != nil || ok {
		t.Fatalf("empty config ok=%v err=%v", ok, err)
	}
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
		if _, ok, err := credentialsFrom(tc.id, tc.hash); err == nil || ok {
			t.Fatalf("credentialsFrom(%q, %q) ok=%v err=%v", tc.id, tc.hash, ok, err)
		}
	}
}
