package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestRecipientJIDParsesPhoneNumbersAndJIDs(t *testing.T) {
	jid, err := recipientJID("+1 (555) 010-2222")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jid.String(), "15550102222@s.whatsapp.net"; got != want {
		t.Fatalf("phone JID = %q, want %q", got, want)
	}
	jid, err = recipientJID("12345-67890@g.us")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jid.String(), "12345-67890@g.us"; got != want {
		t.Fatalf("explicit JID = %q, want %q", got, want)
	}
}

func TestConnectionIDsUsesDurableConnectionRows(t *testing.T) {
	ids := connectionIDs([]integrations.Connection{
		{ID: "whatsapp:15550101111"},
		{ID: ""},
		{ID: "whatsapp:15550102222"},
	})
	if !ids["whatsapp:15550101111"] || !ids["whatsapp:15550102222"] || ids[""] {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestFailQRSessionRemovesProviderSession(t *testing.T) {
	provider := &Provider{sessions: map[string]*qrSession{}}
	session := &qrSession{
		id:     "whatsapp_qr_1",
		status: "pending",
		ready:  make(chan struct{}),
	}
	provider.sessions[session.id] = session

	provider.failQRSession(context.Background(), session, errors.New("save failed"))

	if _, ok := provider.sessions[session.id]; ok {
		t.Fatalf("session %q was not removed", session.id)
	}
	status := session.statusSnapshot()
	if status.Status != "failed" || status.Error != "save failed" {
		t.Fatalf("status = %#v", status)
	}
}

func TestWhatsAppRecordsExposeContactsAndMessages(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	contact := whatsappContactRecord(connection, waTypes.NewJID("15550103333", waTypes.DefaultUserServer), waTypes.ContactInfo{
		FullName: "Alice Example",
		PushName: "Alice",
	})
	if got, want := contact.Kind.Domain(), integrations.RecordDomainContacts; got != want {
		t.Fatalf("contact domain = %q, want %q", got, want)
	}
	var contactRaw map[string]any
	if err := json.Unmarshal(contact.Raw, &contactRaw); err != nil {
		t.Fatal(err)
	}
	if contactRaw["full_name"] != "Alice Example" || contactRaw["phone"] != "15550103333" {
		t.Fatalf("contact raw = %#v", contactRaw)
	}

	occurred := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	message := whatsappMessageRecord(connection, &events.Message{
		Info: waTypes.MessageInfo{
			ID: "wamid.1",
			MessageSource: waTypes.MessageSource{
				Chat:   waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
				Sender: waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
			},
			Timestamp: occurred,
			PushName:  "Alice",
		},
		Message: &waE2E.Message{Conversation: proto.String("hello")},
	})
	if got, want := message.Kind.Domain(), integrations.RecordDomainMessages; got != want {
		t.Fatalf("message domain = %q, want %q", got, want)
	}
	if !message.OccurredAt.Equal(occurred) {
		t.Fatalf("message occurred_at = %s, want %s", message.OccurredAt, occurred)
	}
	var messageRaw map[string]any
	if err := json.Unmarshal(message.Raw, &messageRaw); err != nil {
		t.Fatal(err)
	}
	if messageRaw["text"] != "hello" || messageRaw["sender"] != "15550103333@s.whatsapp.net" {
		t.Fatalf("message raw = %#v", messageRaw)
	}
}
