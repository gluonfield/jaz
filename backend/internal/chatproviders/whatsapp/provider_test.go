package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	waStore "go.mau.fi/whatsmeow/store"
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

func TestNewWhatsAppClientUsesBrowserQRIdentity(t *testing.T) {
	client := newWhatsAppClient(&waStore.Device{})
	if client.QRClientType != whatsmeow.PairClientChrome {
		t.Fatalf("QR client type = %q, want %q", client.QRClientType, whatsmeow.PairClientChrome)
	}
	if got := waStore.DeviceProps.GetPlatformType(); got != waCompanionReg.DeviceProps_CHROME {
		t.Fatalf("platform type = %s, want %s", got, waCompanionReg.DeviceProps_CHROME)
	}
	if got := waStore.DeviceProps.GetOs(); got != "Jaz" {
		t.Fatalf("device os = %q, want Jaz", got)
	}
}

func TestFailQRSessionPreservesFailedStatusUntilPolled(t *testing.T) {
	provider := &Provider{sessions: map[string]*qrSession{}}
	session := &qrSession{
		id:     "whatsapp_qr_1",
		status: "pending",
		ready:  make(chan struct{}),
	}
	provider.sessions[session.id] = session

	provider.failQRSession(context.Background(), session, errors.New("save failed"))

	if _, ok := provider.sessions[session.id]; !ok {
		t.Fatalf("session %q was removed before status was read", session.id)
	}
	status, err := provider.QRStatus(context.Background(), session.id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "failed" || status.Error != "save failed" {
		t.Fatalf("status = %#v", status)
	}
	if _, ok := provider.sessions[session.id]; ok {
		t.Fatalf("session %q was not removed after terminal status", session.id)
	}
}

func TestWhatsAppFirstQRCodeErrorPreservesProviderFailure(t *testing.T) {
	err := whatsappFirstQRCodeError(connections.QRStatus{
		Status: "failed",
		Error:  "connect failed",
	})
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("err = %v", err)
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
	if contactRaw["full_name"] != "Alice Example" ||
		contactRaw["display_name"] != "Alice Example" ||
		contactRaw["phone_number"] != "15550103333" ||
		contactRaw["whatsapp_id"] != "15550103333@s.whatsapp.net" {
		t.Fatalf("contact raw = %#v", contactRaw)
	}
	names, ok := contactRaw["contact_names"].([]any)
	if !ok || len(names) != 2 || names[0] != "Alice Example" || names[1] != "Alice" {
		t.Fatalf("contact names = %#v", contactRaw["contact_names"])
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

func TestWriteRecordsMarksSyncCursor(t *testing.T) {
	raw := &fakeWhatsAppRawSink{}
	store := &fakeWhatsAppStore{}
	provider := &Provider{raw: raw, store: store}
	record := integrations.Record{
		Provider:     "whatsapp",
		AccountID:    "15550102222",
		ConnectionID: "whatsapp:15550102222",
		Kind:         "whatsapp.message",
		ExternalID:   "wamid.1",
		Raw:          json.RawMessage(`{"text":"hello"}`),
	}

	if err := provider.writeRecords(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if len(raw.records) != 1 || raw.records[0].ExternalID != record.ExternalID {
		t.Fatalf("records = %#v", raw.records)
	}
	if store.cursorConnectionID != record.ConnectionID || store.cursor.Kind != whatsappSyncCursorKind {
		t.Fatalf("cursor connection=%q cursor=%#v", store.cursorConnectionID, store.cursor)
	}
}

func TestWhatsAppHistoryRecordsDropOldMessagesAndFullProtoBlob(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	newTimestamp := uint64(now.Add(-time.Hour).Unix())
	oldTimestamp := uint64(now.Add(-(whatsappHistoricalWindow + time.Hour)).Unix())
	syncType := waHistorySync.HistorySync_INITIAL_BOOTSTRAP
	chunk := uint32(2)
	progress := uint32(50)
	conversationID := "15550103333@s.whatsapp.net"
	sync := &waHistorySync.HistorySync{
		SyncType:   &syncType,
		ChunkOrder: &chunk,
		Progress:   &progress,
		Conversations: []*waHistorySync.Conversation{{
			ID: proto.String(conversationID),
			Messages: []*waHistorySync.HistorySyncMsg{
				{Message: whatsappWebInfo("new-id", conversationID, newTimestamp, "new message")},
				{Message: whatsappWebInfo("old-id", conversationID, oldTimestamp, "old message")},
			},
		}},
	}

	records := whatsappHistoryRecords(connection, sync, whatsappHistoryCutoff(now))

	if len(records) != 2 {
		t.Fatalf("records len = %d, want metadata + one message", len(records))
	}
	if records[0].Kind != "whatsapp.history" {
		t.Fatalf("metadata kind = %q", records[0].Kind)
	}
	var metadata map[string]any
	if err := json.Unmarshal(records[0].Raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["conversations"]; ok {
		t.Fatalf("metadata contains full conversations blob: %#v", metadata)
	}
	if records[1].ExternalID != "new-id" {
		t.Fatalf("message external id = %q", records[1].ExternalID)
	}
	var message map[string]any
	if err := json.Unmarshal(records[1].Raw, &message); err != nil {
		t.Fatal(err)
	}
	if message["text"] != "new message" {
		t.Fatalf("message raw = %#v", message)
	}
}

func whatsappWebInfo(id, remoteJID string, timestamp uint64, text string) *waWeb.WebMessageInfo {
	return &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			ID:        proto.String(id),
			RemoteJID: proto.String(remoteJID),
		},
		MessageTimestamp: proto.Uint64(timestamp),
		Message:          &waE2E.Message{Conversation: proto.String(text)},
	}
}

type fakeWhatsAppRawSink struct {
	records []integrations.Record
}

func (s *fakeWhatsAppRawSink) WriteRecords(_ context.Context, records []integrations.Record) error {
	s.records = append(s.records, records...)
	return nil
}

type fakeWhatsAppStore struct {
	cursorConnectionID string
	cursor             integrations.Cursor
}

func (s *fakeWhatsAppStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return nil, nil
}

func (s *fakeWhatsAppStore) SaveConnection(context.Context, integrations.Connection) error {
	return nil
}

func (s *fakeWhatsAppStore) SaveIntegrationCursor(_ context.Context, connectionID string, cursor integrations.Cursor) error {
	s.cursorConnectionID = connectionID
	s.cursor = cursor
	return nil
}
