package whatsapp

import (
	"testing"

	waStore "go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
)

func TestConnectionFromDeviceNeverExposesRawPhoneAsName(t *testing.T) {
	jid := waTypes.NewJID("447598490355", waTypes.DefaultUserServer)

	unnamed, ok := connectionFromDevice(&waStore.Device{ID: &jid})
	if !ok {
		t.Fatal("connection not derived")
	}
	if unnamed.AccountID != "447598490355" {
		t.Fatalf("account id = %q", unnamed.AccountID)
	}
	if unnamed.AccountName != "...0355" {
		t.Fatalf("account name = %q, want redacted phone", unnamed.AccountName)
	}

	named, ok := connectionFromDevice(&waStore.Device{ID: &jid, PushName: "Augustinas"})
	if !ok {
		t.Fatal("connection not derived")
	}
	if named.AccountName != "Augustinas" {
		t.Fatalf("account name = %q", named.AccountName)
	}
}
